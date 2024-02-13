package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/balance"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	push_util "github.com/code-payments/code-server/pkg/code/push"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/jupiter"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	swap_validator "github.com/code-payments/code-server/pkg/solana/swapvalidator"
	"github.com/code-payments/code-server/pkg/usdc"
)

func (s *transactionServer) Swap(streamer transactionpb.Transaction_SwapServer) error {
	ctx, cancel := context.WithTimeout(streamer.Context(), s.conf.swapTimeout.Get(streamer.Context()))
	defer cancel()

	log := s.log.WithField("method", "Swap")
	log = log.WithContext(ctx)
	log = client.InjectLoggingMetadata(ctx, log)

	if s.swapSubsidizer == nil {
		log.Warn("swap subsidizer is not configured")
		return handleSwapError(streamer, status.Error(codes.Unavailable, ""))
	}

	req, err := s.boundedSwapRecv(ctx, streamer)
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return err
	}

	// Client starts a swap by sending the initiation request
	initiateReq := req.GetInitiate()
	if initiateReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate is nil"))
	}

	owner, err := common.NewAccountFromProto(initiateReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	signature := initiateReq.Signature
	initiateReq.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, initiateReq, signature); err != nil {
		return err
	}

	//
	// Section: Antispam
	//

	allow, err := s.antispamGuard.AllowSwap(ctx, owner)
	if err != nil {
		return handleSwapError(streamer, err)
	} else if !allow {
		return handleSwapError(streamer, newSwapDeniedError("rate limited"))
	}

	//
	// Section: Swap parameter setup and validation (accounts, balances, etc.)
	//

	swapAuthority, err := common.NewAccountFromProto(initiateReq.SwapAuthority)
	if err != nil {
		log.WithError(err).Warn("invalid swap authority")
		return handleSwapError(streamer, err)
	}

	if owner.PublicKey().ToBase58() == swapAuthority.PublicKey().ToBase58() {
		return handleSwapError(streamer, newSwapValidationError("owner cannot be swap authority"))
	}

	accountInfoRecord, err := s.data.GetAccountInfoByAuthorityAddress(ctx, swapAuthority.PublicKey().ToBase58())
	switch err {
	case nil:
		if accountInfoRecord.AccountType != commonpb.AccountType_SWAP {
			return handleSwapError(streamer, newSwapValidationError("swap authority isn't an authority for a swap account"))
		}

		if accountInfoRecord.OwnerAccount != owner.PublicKey().ToBase58() {
			return handleSwapError(streamer, newSwapValidationError("swap authority isn't linked to owner"))
		}
	case account.ErrAccountInfoNotFound:
		return handleSwapError(streamer, newSwapValidationError("swap authority isn't linked"))
	default:
		log.WithError(err).Warn("failure getting account info record")
		return handleSwapError(streamer, err)
	}

	swapSource, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid usdc ata")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("swap_source", swapSource.PublicKey().ToBase58())

	accountInfoRecord, err = s.data.GetAccountInfoByAuthorityAddress(ctx, owner.PublicKey().ToBase58())
	switch err {
	case nil:
		if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
			// Should never happen if we're doing phone number validation against
			// the owner account.
			return handleSwapError(streamer, newSwapValidationError("owner must be authority to primary account"))
		}
	case account.ErrAccountInfoNotFound:
		return handleSwapError(streamer, newSwapValidationError("must submit open accounts intent"))
	default:
		log.WithError(err).Warn("failure getting account info record")
		return handleSwapError(streamer, err)
	}

	swapDestination, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid kin primary account")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("swap_destination", swapDestination.PublicKey().ToBase58())

	swapSourceBalance, _, err := balance.CalculateFromBlockchain(ctx, s.data, swapSource)
	if err != nil {
		log.WithError(err).Warn("failure getting swap source account balance")
		return handleSwapError(streamer, err)
	}

	var amountToSwap uint64
	if initiateReq.Limit == 0 {
		amountToSwap = swapSourceBalance
	} else {
		amountToSwap = initiateReq.Limit
	}
	if amountToSwap == 0 {
		return handleSwapError(streamer, newSwapValidationError("usdc account balance is 0"))
	} else if swapSourceBalance < amountToSwap {
		return handleSwapError(streamer, newSwapValidationError("insufficient usdc balance"))
	}
	log = log.WithField("amount_to_swap", amountToSwap)

	//
	// Section: Jupiter routing
	//

	quote, err := s.jupiterClient.GetQuote(
		ctx,
		usdc.Mint,
		kin.Mint,
		amountToSwap,
		50,   // todo: configurable slippage or something based on liquidity?
		true, // Direct routes for now since we're using legacy instructions
		16,   // Max accounts limited due to the use of legacy instructions
		true, // Force legacy instructions
	)
	if err != nil {
		log.WithError(err).Warn("failure getting quote from jupiter")
		return handleSwapError(streamer, err)
	}

	jupiterSwapIxns, err := s.jupiterClient.GetSwapInstructions(
		ctx,
		quote,
		swapAuthority.PublicKey().ToBase58(),
		swapDestination.PublicKey().ToBase58(),
	)
	if err != nil {
		log.WithError(err).Warn("failure getting swap instructions from jupiter")
		return handleSwapError(streamer, err)
	}

	log = log.WithField("estimated_amount_to_receive", quote.GetEstimatedSwapAmount())

	//
	// Section: Validation
	//

	if err := s.validateSwap(ctx, amountToSwap, quote, jupiterSwapIxns); err != nil {
		switch err.(type) {
		case SwapValidationError:
			log.WithError(err).Warn("swap failed validation")
		default:
			log.WithError(err).Warn("failure performing swap validation")
		}
		return handleSwapError(streamer, err)
	}

	//
	// Section: Transaction construction
	//

	swapNonce, err := common.NewRandomAccount()
	if err != nil {
		log.WithError(err).Warn("failure generating swap nonce")
		return handleSwapError(streamer, err)
	}

	preSwapState, preSwapStateBump, err := swap_validator.GetPreSwapStateAddress(&swap_validator.GetPreSwapStateAddressArgs{
		Source:      swapSource.PublicKey().ToBytes(),
		Destination: swapDestination.PublicKey().ToBytes(),
		Nonce:       swapNonce.PublicKey().ToBytes(),
	})
	if err != nil {
		log.WithError(err).Warn("failure deriving pre swap state account address")
		return handleSwapError(streamer, err)
	}

	var remainingAccountsToValidate []swap_validator.AccountMeta
	for _, accountMeta := range jupiterSwapIxns.SwapInstruction.Accounts {
		if accountMeta.IsWritable || accountMeta.IsSigner {
			if bytes.Equal(accountMeta.PublicKey, swapAuthority.PublicKey().ToBytes()) ||
				bytes.Equal(accountMeta.PublicKey, swapSource.PublicKey().ToBytes()) ||
				bytes.Equal(accountMeta.PublicKey, swapDestination.PublicKey().ToBytes()) {
				continue
			}

			remainingAccountsToValidate = append(remainingAccountsToValidate, swap_validator.AccountMeta{
				PublicKey: accountMeta.PublicKey,
			})
		}
	}

	preSwapIxn := swap_validator.NewPreSwapInstruction(
		&swap_validator.PreSwapInstructionAccounts{
			PreSwapState:      preSwapState,
			User:              swapAuthority.PublicKey().ToBytes(),
			Source:            swapSource.PublicKey().ToBytes(),
			Destination:       swapDestination.PublicKey().ToBytes(),
			Nonce:             swapNonce.PublicKey().ToBytes(),
			Payer:             s.swapSubsidizer.PublicKey().ToBytes(),
			RemainingAccounts: remainingAccountsToValidate,
		},
		&swap_validator.PreSwapInstructionArgs{},
	).ToLegacyInstruction()

	postSwapIxn := swap_validator.NewPostSwapInstruction(
		&swap_validator.PostSwapInstructionAccounts{
			PreSwapState: preSwapState,
			Source:       swapSource.PublicKey().ToBytes(),
			Destination:  swapDestination.PublicKey().ToBytes(),
			Payer:        s.swapSubsidizer.PublicKey().ToBytes(),
		},
		&swap_validator.PostSwapInstructionArgs{
			StateBump:    preSwapStateBump,
			MaxToSend:    amountToSwap,
			MinToReceive: quote.GetEstimatedSwapAmount(),
		},
	).ToLegacyInstruction()

	var ixns []solana.Instruction
	ixns = append(ixns, jupiterSwapIxns.ComputeBudgetInstructions...)
	ixns = append(ixns, preSwapIxn, jupiterSwapIxns.SwapInstruction, postSwapIxn)

	txn := solana.NewTransaction(s.swapSubsidizer.PublicKey().ToBytes(), ixns...)

	blockhash, err := s.data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		log.WithError(err).Warn("failure getting latest blockhash")
		return handleSwapError(streamer, err)
	}
	txn.SetBlockhash(blockhash)

	//
	// Section: Server parameters
	//

	computeUnitLimit, _ := compute_budget.DecompileSetComputeUnitLimitIxnData(jupiterSwapIxns.ComputeBudgetInstructions[0].Data)
	computeUnitPrice, _ := compute_budget.DecompileSetComputeUnitPriceIxnData(jupiterSwapIxns.ComputeBudgetInstructions[1].Data)

	var protoSwapIxnAccounts []*commonpb.InstructionAccount
	for _, ixnAccount := range jupiterSwapIxns.SwapInstruction.Accounts {
		protoSwapIxnAccounts = append(protoSwapIxnAccounts, &commonpb.InstructionAccount{
			Account:    &commonpb.SolanaAccountId{Value: ixnAccount.PublicKey},
			IsSigner:   ixnAccount.IsSigner,
			IsWritable: ixnAccount.IsWritable,
		})
	}

	// Server responds back with parameters, so client can locally construct the
	// transaction and validate it.
	serverParameters := &transactionpb.SwapResponse{
		Response: &transactionpb.SwapResponse_ServerParameters_{
			ServerParameters: &transactionpb.SwapResponse_ServerParameters{
				Payer:            s.swapSubsidizer.ToProto(),
				RecentBlockhash:  &commonpb.Blockhash{Value: blockhash[:]},
				ComputeUnitLimit: computeUnitLimit,
				ComputeUnitPrice: computeUnitPrice,
				SwapProgram:      &commonpb.SolanaAccountId{Value: jupiterSwapIxns.SwapInstruction.Program},
				SwapIxnAccounts:  protoSwapIxnAccounts,
				SwapIxnData:      jupiterSwapIxns.SwapInstruction.Data,
				MaxToSend:        amountToSwap,
				MinToReceive:     quote.GetEstimatedSwapAmount(),
				Nonce:            swapNonce.ToProto(),
			},
		},
	}
	if err := streamer.Send(serverParameters); err != nil {
		return handleSwapError(streamer, err)
	}

	//
	// Section: Transaction signing
	//

	req, err = s.boundedSwapRecv(ctx, streamer)
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return err
	}

	// Client responds back with a signatures to the swap transaction
	submitSignatureReq := req.GetSubmitSignature()
	if submitSignatureReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.SubmitSignature is nil"))
	}

	if !ed25519.Verify(
		swapAuthority.PublicKey().ToBytes(),
		txn.Message.Marshal(),
		submitSignatureReq.Signature.Value,
	) {
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_SIGNATURE_ERROR,
			toInvalidSignatureErrorDetails(0, txn, submitSignatureReq.Signature),
		)
	}

	copy(txn.Signatures[clientSignatureIndex][:], submitSignatureReq.Signature.Value)
	txn.Sign(s.swapSubsidizer.PrivateKey().ToBytes())

	log = log.WithField("transaction_id", base58.Encode(txn.Signature()))

	//
	// Section: Transaction submission
	//

	_, err = s.data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		log.WithError(err).Warn("failure submitting transaction")
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_SWAP_FAILED,
			toReasonStringErrorDetails(err),
		)
	}

	log.Debug("submitted transaction")

	s.bestEffortNotifyUserOfSwapInProgress(ctx, owner)

	if !initiateReq.WaitForBlockchainStatus {
		err = streamer.Send(&transactionpb.SwapResponse{
			Response: &transactionpb.SwapResponse_Success_{
				Success: &transactionpb.SwapResponse_Success{
					Code: transactionpb.SwapResponse_Success_SWAP_SUBMITTED,
				},
			},
		})
		return handleSwapError(streamer, err)
	}

	for {
		select {
		case <-time.After(time.Second):
			statuses, err := s.data.GetBlockchainSignatureStatuses(ctx, []solana.Signature{solana.Signature(txn.Signature())})
			if err != nil {
				continue
			}

			if len(statuses) == 0 || statuses[0] == nil {
				continue
			}

			if statuses[0].ErrorResult != nil {
				log.WithError(statuses[0].ErrorResult).Warn("transaction failed")
				return handleSwapStructuredError(streamer, transactionpb.SwapResponse_Error_SWAP_FAILED)
			}

			if statuses[0].Finalized() {
				log.Debug("transaction succeeded and is finalized")
				err = streamer.Send(&transactionpb.SwapResponse{
					Response: &transactionpb.SwapResponse_Success_{
						Success: &transactionpb.SwapResponse_Success{
							Code: transactionpb.SwapResponse_Success_SWAP_FINALIZED,
						},
					},
				})
				return handleSwapError(streamer, err)
			}
		case <-ctx.Done():
			return handleSwapError(streamer, ctx.Err())
		}
	}
}

// Temporary for manual USDC deposit flow
func (s *transactionServer) bestEffortNotifyUserOfSwapInProgress(ctx context.Context, owner *common.Account) {
	chatMessage, err := chat_util.NewUsdcBeingConvertedMessage()
	if err != nil {
		return
	}

	canPush, err := chat_util.SendCodeTeamMessage(ctx, s.data, owner, chatMessage)
	if err != nil {
		return
	}

	if canPush {
		push_util.SendChatMessagePushNotification(
			ctx,
			s.data,
			s.pusher,
			chat_util.CodeTeamName,
			owner,
			chatMessage,
		)
	}
}

func (s *transactionServer) validateSwap(
	ctx context.Context,
	amountToSwap uint64,
	quote *jupiter.Quote,
	ixns *jupiter.SwapInstructions,
) error {
	//
	// Part 1: Expected instructions sanity check
	//

	if len(ixns.ComputeBudgetInstructions) != 2 {
		return newSwapValidationError("expected two compute budget instructions")
	}

	if ixns.TokenLedgerInstruction != nil || ixns.CleanupInstruction != nil {
		return newSwapValidationError("unexpected instruction")
	}

	//
	// Part 2: Compute budget instructions
	//

	if !bytes.Equal(ixns.ComputeBudgetInstructions[0].Program, compute_budget.ProgramKey) || !bytes.Equal(ixns.ComputeBudgetInstructions[1].Program, compute_budget.ProgramKey) {
		return newSwapValidationError("invalid ComputeBudget program key")
	}

	if len(ixns.ComputeBudgetInstructions[0].Accounts) != 0 || len(ixns.ComputeBudgetInstructions[1].Accounts) != 0 {
		return newSwapValidationError("invalid ComputeBudget instruction accounts")
	}

	if _, err := compute_budget.DecompileSetComputeUnitLimitIxnData(ixns.ComputeBudgetInstructions[0].Data); err != nil {
		return newSwapValidationErrorf("invalid ComputeBudget::SetComputeUnitLimit instruction data: %s", err.Error())
	}

	if _, err := compute_budget.DecompileSetComputeUnitPriceIxnData(ixns.ComputeBudgetInstructions[1].Data); err != nil {
		return newSwapValidationErrorf("invalid ComputeBudget::SetComputeUnitPrice instruction data: %s", err.Error())
	}

	//
	// Part 3: Swap instruction
	//

	for _, ixnAccount := range ixns.SwapInstruction.Accounts {
		if bytes.Equal(ixnAccount.PublicKey, s.swapSubsidizer.PublicKey().ToBytes()) {
			return newSwapValidationError("swap subsidizer used in swap instruction")
		}
	}

	usdcAmount := float64(amountToSwap) / float64(usdc.QuarksPerUsdc)
	kinAmount := float64(quote.GetEstimatedSwapAmount()) / float64(kin.QuarksPerKin)
	swapRate := usdcAmount / kinAmount

	usdExchangeRateRateRecord, err := s.data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
	if err != nil {
		return errors.Wrap(err, "error getting usd exchange rate record")
	}

	// todo: configurable
	swapRateThreshold := 1.25
	if swapRate/usdExchangeRateRateRecord.Rate > swapRateThreshold {
		return newSwapValidationErrorf("swap rate exceeds current exchange rate by %.2fx", swapRateThreshold)
	}

	return nil
}

func (s *transactionServer) mustLoadSwapSubsidizer(ctx context.Context) {
	log := s.log.WithFields(logrus.Fields{
		"method": "mustLoadSwapSubsidizer",
		"key":    s.conf.swapSubsidizerOwnerPublicKey.Get(ctx),
	})

	err := func() error {
		vaultRecord, err := s.data.GetKey(ctx, s.conf.swapSubsidizerOwnerPublicKey.Get(ctx))
		if err != nil {
			return err
		}

		ownerAccount, err := common.NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
		if err != nil {
			return err
		}

		s.swapSubsidizer = ownerAccount
		return nil
	}()
	if err != nil {
		log.WithError(err).Fatal("failure loading account")
	}
}

func (s *transactionServer) boundedSwapRecv(ctx context.Context, streamer transactionpb.Transaction_SwapServer) (req *transactionpb.SwapRequest, err error) {
	done := make(chan struct{})
	go func() {
		req, err = streamer.Recv()
		close(done)
	}()

	select {
	case <-time.After(s.conf.clientReceiveTimeout.Get(ctx)):
		return nil, ErrTimedOutReceivingRequest
	case <-done:
		return req, err
	}
}
