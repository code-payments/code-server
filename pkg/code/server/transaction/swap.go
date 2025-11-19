package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/solana"
)

// Currently we only support stateless swaps against the Currency Creator program
func (s *transactionServer) Swap(streamer transactionpb.Transaction_SwapServer) error {
	// Bound the total RPC. Keeping the timeout higher to see where we land because
	// there's a lot of stuff happening in this method.
	ctx, cancel := context.WithTimeout(streamer.Context(), s.conf.swapTimeout.Get(streamer.Context()))
	defer cancel()

	log := s.log.WithField("method", "Swap")
	log = log.WithContext(ctx)
	log = client.InjectLoggingMetadata(ctx, log)

	req, err := s.boundedSwapRecv(ctx, streamer)
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return handleSwapError(streamer, err)
	}

	initiateReq := req.GetInitiate()
	if initiateReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate is nil"))
	}

	statelessReq := initiateReq.GetStateless()
	if statelessReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate.Stateless is nil"))
	}

	owner, err := common.NewAccountFromProto(statelessReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	reqSignature := statelessReq.Signature
	statelessReq.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, statelessReq, reqSignature); err != nil {
		return err
	}

	swapAuthority, err := common.NewAccountFromProto(statelessReq.SwapAuthority)
	if err != nil {
		log.WithError(err).Warn("invalid swap authority")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("swap_authority", swapAuthority.PublicKey().ToBase58())

	fromMint, err := common.GetBackwardsCompatMint(statelessReq.FromMint)
	if err != nil {
		log.WithError(err).Warn("invalid mint")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("from_mint", fromMint.PublicKey().ToBase58())

	toMint, err := common.GetBackwardsCompatMint(statelessReq.ToMint)
	if err != nil {
		log.WithError(err).Warn("invalid mint")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("to_mint", toMint.PublicKey().ToBase58())

	//
	// Section: Antispam
	//

	ownerManagemntState, err := common.GetOwnerManagementState(ctx, s.data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return handleSwapError(streamer, err)
	}
	if ownerManagemntState != common.OwnerManagementStateCodeAccount {
		return handleSwapError(streamer, NewSwapDeniedError("not a code account"))
	}

	allow, err := s.antispamGuard.AllowSwap(ctx, owner, fromMint, toMint)
	if err != nil {
		return handleSwapError(streamer, err)
	} else if !allow {
		return handleSwapError(streamer, NewSwapDeniedError("rate limited"))
	}

	//
	// Section: Validation
	//

	if owner.PublicKey().ToBase58() == swapAuthority.PublicKey().ToBase58() {
		return handleSwapError(streamer, NewSwapValidationError("owner cannot be swap authority"))
	}

	if bytes.Equal(fromMint.PublicKey().ToBytes(), toMint.PublicKey().ToBytes()) {
		return handleSwapError(streamer, NewSwapValidationError("must swap between two different mints"))
	}

	if statelessReq.Amount == 0 {
		return handleSwapError(streamer, NewSwapValidationError("amount must be positive"))
	}

	sourceVmConfig, err := common.GetVmConfigForMint(ctx, s.data, fromMint)
	if err == common.ErrUnsupportedMint {
		return handleSwapError(streamer, NewSwapValidationError("invalid source mint"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting source vm config")
		return handleSwapError(streamer, err)
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, s.data, toMint)
	if err == common.ErrUnsupportedMint {
		return handleSwapError(streamer, NewSwapValidationError("invalid destination mint"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting destination vm config")
		return handleSwapError(streamer, err)
	}

	ownerDestinationTimelockVault, err := owner.ToTimelockVault(destinationVmConfig)
	if err != nil {
		log.WithError(err).Warn("failure getting owner destination timelock accounts")
		return handleSwapError(streamer, err)
	}
	ownerDestinationTimelockRecord, err := s.data.GetTimelockByVault(ctx, ownerDestinationTimelockVault.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		return handleSwapError(streamer, NewSwapValidationError("destination timelock vault account not opened"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting destination timelock record")
		return handleSwapError(streamer, err)
	}

	//
	// Section: On-demand account creation
	//

	// todo: commonalities here and in geyser external deposit flow
	// todo: this will live somewhere else once we add the state management system
	if !ownerDestinationTimelockRecord.ExistsOnBlockchain() {
		err = ensureVirtualTimelockAccountIsInitialzed(ctx, s.data, ownerDestinationTimelockRecord)
		if err != nil {
			log.WithError(err).Warn("failure scheduling destination timelock account initialization")
			return handleSwapError(streamer, err)
		}

		for range 60 {
			time.Sleep(time.Second)

			_, _, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, s.vmIndexerClient, destinationVmConfig.Vm, owner)
			if err == nil {
				break
			}
		}
		if err != nil {
			log.WithError(err).Warn("timed out waiting for destination timelock account initialization")
			return handleSwapError(streamer, err)
		}
	}

	//
	// Section: Transaction construction
	//

	var swapHandler SwapHandler
	if common.IsCoreMint(fromMint) {
		swapHandler = NewCurrencyCreatorBuySwapHandler(
			s.data,
			s.vmIndexerClient,
			owner,
			swapAuthority,
			toMint,
			statelessReq.Amount,
		)
	} else if common.IsCoreMint(toMint) {
		swapHandler = NewCurrencyCreatorSellSwapHandler(
			s.data,
			s.vmIndexerClient,
			owner,
			swapAuthority,
			fromMint,
			statelessReq.Amount,
		)
	} else {
		swapHandler = NewCurrencyCreatorBuySellSwapHandler(
			s.data,
			s.vmIndexerClient,
			owner,
			swapAuthority,
			fromMint,
			toMint,
			statelessReq.Amount,
		)
	}

	var alts []solana.AddressLookupTable
	for _, mint := range []*common.Account{fromMint, toMint} {
		if common.IsCoreMint(mint) {
			continue
		}

		alt, err := transaction_util.GetAltForMint(ctx, s.data, mint)
		if err != nil {
			log.WithError(err).Warn("failure getting alt")
			return handleSwapError(streamer, err)
		}
		alts = append(alts, alt)
	}

	ixns, err := swapHandler.MakeInstructions(ctx)
	if err != nil {
		log.WithError(err).Warn("failure making instructions")
		return handleSwapError(streamer, err)
	}

	txn := solana.NewV0Transaction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		alts,
		ixns,
	)

	blockhash, err := s.data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		log.WithError(err).Warn("failure getting latest blockhash")
		return handleSwapError(streamer, err)
	}
	txn.SetBlockhash(blockhash)

	marshalledTxn := txn.Marshal()
	marshalledTxnMessage := txn.Message.Marshal()

	//
	// Section: Server parameters
	//

	serverParameters := swapHandler.GetServerParameters()

	protoAlts := make([]*commonpb.SolanaAddressLookupTable, len(alts))
	for i, alt := range alts {
		protoAlts[i] = transaction_util.ToProtoAlt(alt)
	}

	protoServerParameters := &transactionpb.SwapResponse{
		Response: &transactionpb.SwapResponse_ServerParameters_{
			ServerParameters: &transactionpb.SwapResponse_ServerParameters{
				Kind: &transactionpb.SwapResponse_ServerParameters_CurrencyCreator_{
					CurrencyCreator: &transactionpb.SwapResponse_ServerParameters_CurrencyCreator{
						Payer:            common.GetSubsidizer().ToProto(),
						RecentBlockhash:  &commonpb.Blockhash{Value: blockhash[:]},
						Alts:             protoAlts,
						ComputeUnitLimit: serverParameters.ComputeUnitLimit,
						ComputeUnitPrice: serverParameters.ComputeUnitPrice,
						MemoValue:        serverParameters.MemoValue,
						MemoryAccount:    serverParameters.MemoryAccount.ToProto(),
						MemoryIndex:      uint32(serverParameters.MemoryIndex),
					},
				},
			},
		},
	}
	if err := streamer.Send(protoServerParameters); err != nil {
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

	submitSignaturesReq := req.GetSubmitSignatures()
	if submitSignaturesReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.SubmitSignatures is nil"))
	}

	for i := range txn.Message.Header.NumSignatures {
		account := txn.Message.Accounts[i]

		var isClientSignature bool
		var protoSignature *commonpb.Signature

		if bytes.Equal(account, owner.PublicKey().ToBytes()) {
			isClientSignature = true
			protoSignature = submitSignaturesReq.Signatures[0]
		} else if bytes.Equal(account, swapAuthority.PublicKey().ToBytes()) {
			isClientSignature = true
			protoSignature = submitSignaturesReq.Signatures[1]
		}

		if !isClientSignature {
			continue
		}

		if !ed25519.Verify(
			account,
			marshalledTxnMessage,
			protoSignature.Value,
		) {
			return handleSwapStructuredError(
				streamer,
				transactionpb.SwapResponse_Error_SIGNATURE_ERROR,
				toInvalidTxnSignatureErrorDetails(0, txn, protoSignature),
			)
		}

		copy(txn.Signatures[i][:], protoSignature.Value)
	}

	err = txn.Sign(
		common.GetSubsidizer().PrivateKey().ToBytes(),
		sourceVmConfig.Authority.PrivateKey().ToBytes(),
		destinationVmConfig.Authority.PrivateKey().ToBytes(),
	)
	if err != nil {
		log.WithError(err).Info("failure signing transaction")
		return handleSwapError(streamer, err)
	}

	txnSignature := base58.Encode(txn.Signature())
	log = log.WithField("signature", txnSignature)

	//
	// Section: Transaction submission
	//

	log = log.WithField("txn", base64.StdEncoding.EncodeToString(marshalledTxn))

	_, err = s.data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		log.WithError(err).Warn("failure submitting transaction")
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_SWAP_FAILED,
			toReasonStringErrorDetails(err),
		)
	}

	log.Info("transaction submitted")

	//
	// Section: Balance and transaction history
	//

	// todo: commonalities here and in geyser external deposit flow
	// todo: this will live somewhere else once we add the state management system
	var waitForBalanceUpdate sync.WaitGroup
	waitForBalanceUpdate.Add(1)
	go func() {
		defer waitForBalanceUpdate.Done()

		ctx := context.Background()

		var tokenBalances *solana.TransactionTokenBalances
		for range 30 {
			time.Sleep(time.Second)

			tokenBalances, err = s.data.GetBlockchainTransactionTokenBalances(ctx, txnSignature)
			if err == nil {
				break
			}
		}

		if err != nil {
			log.WithError(err).Warn("failure getting transaction token balances")
			return
		}

		deltaQuarksIntoOmnibus, err := getDeltaQuarksFromTokenBalances(destinationVmConfig.Omnibus, tokenBalances)
		if err != nil {
			log.WithError(err).Warn("failure getting delta quarks into destination vm omnibus")
			return
		}
		if deltaQuarksIntoOmnibus <= 0 {
			log.Warn("delta quarks into destination vm omnibus is not positive")
			return
		}

		usdMarketValue, _, err := currency_util.CalculateUsdMarketValue(ctx, s.data, toMint, uint64(deltaQuarksIntoOmnibus), time.Now())
		if err != nil {
			log.WithError(err).Warn("failure calculating usd market value")
			return
		}

		err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
			// For transaction history
			intentRecord := &intent.Record{
				IntentId:   getSwapDepositIntentID(txnSignature, ownerDestinationTimelockVault),
				IntentType: intent.ExternalDeposit,

				MintAccount: toMint.PublicKey().ToBase58(),

				InitiatorOwnerAccount: owner.PublicKey().ToBase58(),

				ExternalDepositMetadata: &intent.ExternalDepositMetadata{
					DestinationTokenAccount: ownerDestinationTimelockVault.PublicKey().ToBase58(),
					Quantity:                uint64(deltaQuarksIntoOmnibus),
					UsdMarketValue:          usdMarketValue,
				},

				State:     intent.StateConfirmed,
				CreatedAt: time.Now(),
			}
			err = s.data.SaveIntent(ctx, intentRecord)
			if err != nil {
				return errors.Wrap(err, "error saving intent record")
			}

			// For tracking in cached balances
			externalDepositRecord := &deposit.Record{
				Signature:      txnSignature,
				Destination:    ownerDestinationTimelockVault.PublicKey().ToBase58(),
				Amount:         uint64(deltaQuarksIntoOmnibus),
				UsdMarketValue: usdMarketValue,

				Slot:              tokenBalances.Slot,
				ConfirmationState: transaction.ConfirmationFinalized,

				CreatedAt: time.Now(),
			}
			err = s.data.SaveExternalDeposit(ctx, externalDepositRecord)
			if err != nil {
				return errors.Wrap(err, "error saving external deposit record")
			}

			return nil
		})
		if err != nil {
			log.WithError(err).Warn("failure updating transaction history and balance")
		}
	}()

	//
	// Section: Final RPC response
	//

	if !statelessReq.WaitForBlockchainStatus {
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

				waitForBalanceUpdate.Wait()

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

func ensureVirtualTimelockAccountIsInitialzed(ctx context.Context, data code_data.Provider, timelockRecord *timelock.Record) error {
	if !timelockRecord.ExistsOnBlockchain() {
		initializeFulfillmentRecord, err := data.GetFirstSchedulableFulfillmentByAddressAsSource(ctx, timelockRecord.VaultAddress)
		if err != nil {
			return err
		}

		if initializeFulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
			return errors.New("expected an initialize locked timelock account fulfillment")
		}

		return markFulfillmentAsActivelyScheduled(ctx, data, initializeFulfillmentRecord)
	}

	return nil
}

func markFulfillmentAsActivelyScheduled(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.Id == 0 {
		return nil
	}

	if !fulfillmentRecord.DisableActiveScheduling {
		return nil
	}

	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return nil
	}

	fulfillmentRecord.DisableActiveScheduling = false
	return data.UpdateFulfillment(ctx, fulfillmentRecord)
}

func getDeltaQuarksFromTokenBalances(tokenAccount *common.Account, tokenBalances *solana.TransactionTokenBalances) (int64, error) {
	var preQuarkBalance, postQuarkBalance int64
	var err error
	for _, tokenBalance := range tokenBalances.PreTokenBalances {
		if tokenBalances.Accounts[tokenBalance.AccountIndex] == tokenAccount.PublicKey().ToBase58() {
			preQuarkBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "error parsing pre token balance")
			}
			break
		}
	}
	for _, tokenBalance := range tokenBalances.PostTokenBalances {
		if tokenBalances.Accounts[tokenBalance.AccountIndex] == tokenAccount.PublicKey().ToBase58() {
			postQuarkBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "error parsing post token balance")
			}
			break
		}
	}

	return postQuarkBalance - preQuarkBalance, nil
}

// Consistent intent ID that maps to a 32 byte buffer
func getSwapDepositIntentID(signature string, destination *common.Account) string {
	combined := fmt.Sprintf("%s-%s", signature, destination.PublicKey().ToBase58())
	hashed := sha256.Sum256([]byte(combined))
	return base58.Encode(hashed[:])
}
