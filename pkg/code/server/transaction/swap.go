package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"time"

	"github.com/mr-tron/base58/base58"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/token"
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

	signature := statelessReq.Signature
	statelessReq.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, statelessReq, signature); err != nil {
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

		alt, err := transaction.GetAltForMint(ctx, s.data, fromMint)
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

	//
	// Section: Server parameters
	//

	serverParameters := swapHandler.GetServerParameters()

	protoAlts := make([]*commonpb.SolanaAddressLookupTable, len(alts))
	for i, alt := range alts {
		protoAlts[i] = transaction.ToProtoAlt(alt)
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
			marshalledTxn,
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
	log = log.WithField("signature", base58.Encode(txn.Signature()))

	//
	// Section: Transaction submission
	//

	log = log.WithField("txn", base64.StdEncoding.EncodeToString(txn.Marshal()))

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

type SwapServerParameters struct {
	ComputeUnitLimit uint32
	ComputeUnitPrice uint64
	MemoValue        string
	MemoryAccount    *common.Account
	MemoryIndex      uint16
}

type SwapHandler interface {
	GetServerParameters() *SwapServerParameters

	MakeInstructions(ctx context.Context) ([]solana.Instruction, error)
}

type CurrencyCreatorBuySwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	buyer           *common.Account
	temporaryHolder *common.Account
	mint            *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorBuySwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	buyer *common.Account,
	temporaryHolder *common.Account,
	mint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorBuySwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		computeUnitLimit: 300_000,
		computeUnitPrice: 1_000,
		memoValue:        "buy_v0",
	}
}

func (h *CurrencyCreatorBuySwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorBuySwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.buyer.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.mint)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.buyer)
	if err != nil {
		return nil, err
	}

	destinationCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.mint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	destinationCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(destinationCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	createTemporaryCoreMintAtaIxn, temporaryCoreMintAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		common.CoreMintAccount.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporaryCoreMintAta, err := common.NewAccountFromPublicKeyBytes(temporaryCoreMintAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.buyer.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporaryCoreMintAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	buyAndDepositIntoDestinationVmIxn := currencycreator.NewBuyAndDepositIntoVmInstruction(
		&currencycreator.BuyAndDepositIntoVmInstructionAccounts{
			Buyer:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:        destinationCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:    destinationCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:  h.mint.PublicKey().ToBytes(),
			BaseMint:    common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget: destinationCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:   destinationCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			BuyerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:   destinationCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:     destinationCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.buyer.PublicKey().ToBytes(),
		},
		&currencycreator.BuyAndDepositIntoVmInstructionArgs{
			InAmount:      h.amount,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporaryCoreMintAtaIxn := token.CloseAccount(
		temporaryCoreMintAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.buyer.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporaryCoreMintAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		buyAndDepositIntoDestinationVmIxn,
		closeTemporaryCoreMintAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}

type CurrencyCreatorSellSwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	seller          *common.Account
	temporaryHolder *common.Account
	mint            *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorSellSwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	seller *common.Account,
	temporaryHolder *common.Account,
	mint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorSellSwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		seller:          seller,
		temporaryHolder: temporaryHolder,
		mint:            mint,
		amount:          amount,

		computeUnitLimit: 300_000,
		computeUnitPrice: 1_000,
		memoValue:        "sell_v0",
	}
}

func (h *CurrencyCreatorSellSwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorSellSwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.mint)
	if err != nil {
		return nil, err
	}

	sourceCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.mint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(sourceCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.seller.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.seller)
	if err != nil {
		return nil, err
	}

	createTemporarySourceCurrencyAtaIxn, temporarySourceCurrencyAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		h.mint.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporarySourceCurrencyAta, err := common.NewAccountFromPublicKeyBytes(temporarySourceCurrencyAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.seller.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporarySourceCurrencyAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	sellAndDepositIntoDestinationVmIxn := currencycreator.NewSellAndDepositIntoVmInstruction(
		&currencycreator.SellAndDepositIntoVmInstructionAccounts{
			Seller:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:         sourceCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:     sourceCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:   h.mint.PublicKey().ToBytes(),
			BaseMint:     common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget:  sourceCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:    sourceCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			SellerTarget: temporarySourceCurrencyAta.PublicKey().ToBytes(),
			FeeTarget:    sourceCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:      sourceCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.seller.PublicKey().ToBytes(),
		},
		&currencycreator.SellAndDepositIntoVmInstructionArgs{
			InAmount:      h.amount,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporarySourceCurrencyAtaIxn := token.CloseAccount(
		temporarySourceCurrencyAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.seller.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporarySourceCurrencyAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		sellAndDepositIntoDestinationVmIxn,
		closeTemporarySourceCurrencyAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}

type CurrencyCreatorBuySellSwapHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient

	swapper         *common.Account
	temporaryHolder *common.Account
	fromMint        *common.Account
	toMint          *common.Account
	amount          uint64

	computeUnitLimit uint32
	computeUnitPrice uint64
	memoValue        string
	memoryAccount    *common.Account
	memoryIndex      uint16
}

func NewCurrencyCreatorBuySellSwapHandler(
	data code_data.Provider,
	vmIndexerClient indexerpb.IndexerClient,
	swapper *common.Account,
	temporaryHolder *common.Account,
	fromMint *common.Account,
	toMint *common.Account,
	amount uint64,
) SwapHandler {
	return &CurrencyCreatorBuySellSwapHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,

		swapper:         swapper,
		temporaryHolder: temporaryHolder,
		fromMint:        fromMint,
		toMint:          toMint,
		amount:          amount,

		computeUnitLimit: 400_000,
		computeUnitPrice: 1_000,
		memoValue:        "buy_sell_v0",
	}
}

func (h *CurrencyCreatorBuySellSwapHandler) GetServerParameters() *SwapServerParameters {
	return &SwapServerParameters{
		ComputeUnitLimit: h.computeUnitLimit,
		ComputeUnitPrice: h.computeUnitPrice,
		MemoValue:        h.memoValue,
		MemoryAccount:    h.memoryAccount,
		MemoryIndex:      h.memoryIndex,
	}
}

func (h *CurrencyCreatorBuySellSwapHandler) MakeInstructions(ctx context.Context) ([]solana.Instruction, error) {
	sourceVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.fromMint)
	if err != nil {
		return nil, err
	}

	sourceCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.fromMint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(sourceCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	sourceTimelockAccounts, err := h.swapper.GetTimelockAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, h.data, h.toMint)
	if err != nil {
		return nil, err
	}

	h.memoryAccount, h.memoryIndex, err = common.GetVirtualTimelockAccountLocationInMemory(ctx, h.vmIndexerClient, destinationVmConfig.Vm, h.swapper)
	if err != nil {
		return nil, err
	}

	destinationCurrencyMetadataRecord, err := h.data.GetCurrencyMetadata(ctx, h.toMint.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	destinationCurrencyAccounts, err := common.GetLaunchpadCurrencyAccounts(destinationCurrencyMetadataRecord)
	if err != nil {
		return nil, err
	}

	createTemporaryCoreMintAtaIxn, temporaryCoreMintAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		common.CoreMintAccount.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporaryCoreMintAta, err := common.NewAccountFromPublicKeyBytes(temporaryCoreMintAtaBytes)
	if err != nil {
		return nil, err
	}

	createTemporarySourceCurrencyAtaIxn, temporarySourceCurrencyAtaBytes, err := token.CreateAssociatedTokenAccountIdempotent(
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
		h.fromMint.PublicKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}
	temporarySourceCurrencyAta, err := common.NewAccountFromPublicKeyBytes(temporarySourceCurrencyAtaBytes)
	if err != nil {
		return nil, err
	}

	transferFromSourceVmSwapAtaIxn := cvm.NewTransferForSwapInstruction(
		&cvm.TransferForSwapInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.swapper.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: temporarySourceCurrencyAta.PublicKey().ToBytes(),
		},
		&cvm.TransferForSwapInstructionArgs{
			Amount: h.amount,
			Bump:   sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	sellIxn := currencycreator.NewSellTokensInstruction(
		&currencycreator.SellTokensInstructionAccounts{
			Seller:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:         sourceCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:     sourceCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:   h.fromMint.PublicKey().ToBytes(),
			BaseMint:     common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget:  sourceCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:    sourceCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			SellerTarget: temporarySourceCurrencyAta.PublicKey().ToBytes(),
			SellerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:    sourceCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:      sourceCurrencyAccounts.FeesBase.PublicKey().ToBytes(),
		},
		&currencycreator.SellTokensInstructionArgs{
			InAmount:     h.amount,
			MinAmountOut: 0,
		},
	)

	buyAndDepositIntoDestinationVmIxn := currencycreator.NewBuyAndDepositIntoVmInstruction(
		&currencycreator.BuyAndDepositIntoVmInstructionAccounts{
			Buyer:       h.temporaryHolder.PublicKey().ToBytes(),
			Pool:        destinationCurrencyAccounts.LiquidityPool.PublicKey().ToBytes(),
			Currency:    destinationCurrencyAccounts.CurrencyConfig.PublicKey().ToBytes(),
			TargetMint:  h.toMint.PublicKey().ToBytes(),
			BaseMint:    common.CoreMintAccount.PublicKey().ToBytes(),
			VaultTarget: destinationCurrencyAccounts.VaultMint.PublicKey().ToBytes(),
			VaultBase:   destinationCurrencyAccounts.VaultBase.PublicKey().ToBytes(),
			BuyerBase:   temporaryCoreMintAta.PublicKey().ToBytes(),
			FeeTarget:   destinationCurrencyAccounts.FeesMint.PublicKey().ToBytes(),
			FeeBase:     destinationCurrencyAccounts.FeesBase.PublicKey().ToBytes(),

			VmAuthority: destinationVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          destinationVmConfig.Vm.PublicKey().ToBytes(),
			VmMemory:    h.memoryAccount.PublicKey().ToBytes(),
			VmOmnibus:   destinationVmConfig.Omnibus.PublicKey().ToBytes(),
			VtaOwner:    h.swapper.PublicKey().ToBytes(),
		},
		&currencycreator.BuyAndDepositIntoVmInstructionArgs{
			InAmount:      0,
			MinOutAmount:  0,
			VmMemoryIndex: h.memoryIndex,
		},
	)

	closeTemporaryCoreMintAtaIxn := token.CloseAccount(
		temporaryCoreMintAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeTemporarySourceCurrencyAtaIxn := token.CloseAccount(
		temporarySourceCurrencyAta.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
		h.temporaryHolder.PublicKey().ToBytes(),
	)

	closeSourceVmSwapAccountIfEmptyIxn := cvm.NewCloseSwapAccountIfEmptyInstruction(
		&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
			VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
			Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
			Swapper:     h.swapper.PublicKey().ToBytes(),
			SwapPda:     sourceTimelockAccounts.VmSwapAccounts.Pda.PublicKey().ToBytes(),
			SwapAta:     sourceTimelockAccounts.VmSwapAccounts.Ata.PublicKey().ToBytes(),
			Destination: common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&cvm.CloseSwapAccountIfEmptyInstructionArgs{
			Bump: sourceTimelockAccounts.VmSwapAccounts.PdaBump,
		},
	)

	return []solana.Instruction{
		compute_budget.SetComputeUnitLimit(h.computeUnitLimit),
		compute_budget.SetComputeUnitPrice(h.computeUnitPrice),
		memo.Instruction(h.memoValue),
		createTemporaryCoreMintAtaIxn,
		createTemporarySourceCurrencyAtaIxn,
		transferFromSourceVmSwapAtaIxn,
		sellIxn,
		buyAndDepositIntoDestinationVmIxn,
		closeTemporaryCoreMintAtaIxn,
		closeTemporarySourceCurrencyAtaIxn,
		closeSourceVmSwapAccountIfEmptyIxn,
	}, nil
}
