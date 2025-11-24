package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/protoutil"
	"github.com/code-payments/code-server/pkg/solana"
)

func (s *transactionServer) StartSwap(streamer transactionpb.Transaction_StartSwapServer) error {
	ctx := streamer.Context()

	log := s.log.WithField("method", "StartSwap")
	log = client.InjectLoggingMetadata(ctx, log)

	if s.conf.disableSwaps.Get(ctx) {
		return handleStartSwapError(streamer, status.Error(codes.Unavailable, "temporarily unavailable"))
	}

	req, err := protoutil.BoundedReceive[transactionpb.StartSwapRequest](ctx, streamer, s.conf.clientReceiveTimeout.Get(ctx))
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return handleStartSwapError(streamer, err)
	}

	startReq := req.GetStart()
	if startReq == nil {
		return handleStartSwapError(streamer, status.Error(codes.InvalidArgument, "StartSwapRequest.Start is nil"))
	}

	owner, err := common.NewAccountFromProto(startReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return handleStartSwapError(streamer, err)
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	reqSignature := startReq.Signature
	startReq.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, startReq, reqSignature); err != nil {
		return handleStartSwapError(streamer, err)
	}

	startCurrencyCreatorSwapReq := startReq.GetCurrencyCreator()
	if startCurrencyCreatorSwapReq == nil {
		return handleStartSwapError(streamer, status.Error(codes.InvalidArgument, "StartSwapRequest.Start.CurrencyCreator is nil"))
	}

	swapId := base58.Encode(startCurrencyCreatorSwapReq.Id.Value)
	log = log.WithField("swap_id", swapId)

	fromMint, err := common.NewAccountFromProto(startCurrencyCreatorSwapReq.FromMint)
	if err != nil {
		log.WithError(err).Warn("invalid source mint account")
		return handleStartSwapError(streamer, err)
	}

	toMint, err := common.NewAccountFromProto(startCurrencyCreatorSwapReq.ToMint)
	if err != nil {
		log.WithError(err).Warn("invalid destination mint account")
		return handleStartSwapError(streamer, err)
	}

	//
	// Section: Antispam
	//

	ownerManagemntState, err := common.GetOwnerManagementState(ctx, s.data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting owner management state")
		return handleStartSwapError(streamer, err)
	}
	if ownerManagemntState != common.OwnerManagementStateCodeAccount {
		return handleStartSwapError(streamer, NewSwapDeniedError("not a code account"))
	}

	allow, err := s.antispamGuard.AllowSwap(ctx, owner, fromMint, toMint)
	if err != nil {
		return handleStartSwapError(streamer, err)
	} else if !allow {
		return handleStartSwapError(streamer, NewSwapDeniedError("rate limited"))
	}

	//
	// Section: Validation
	//

	if bytes.Equal(fromMint.PublicKey().ToBytes(), toMint.PublicKey().ToBytes()) {
		return handleStartSwapError(streamer, NewSwapValidationError("must swap between two different mints"))
	}

	if startCurrencyCreatorSwapReq.Amount == 0 {
		return handleStartSwapError(streamer, NewSwapValidationError("amount must be positive"))
	}

	_, err = common.GetVmConfigForMint(ctx, s.data, fromMint)
	if err == common.ErrUnsupportedMint {
		return handleStartSwapError(streamer, NewSwapValidationError("invalid source mint"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting source vm config")
		return handleStartSwapError(streamer, err)
	}

	_, err = common.GetVmConfigForMint(ctx, s.data, toMint)
	if err == common.ErrUnsupportedMint {
		return handleStartSwapError(streamer, NewSwapValidationError("invalid destination mint"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting destination vm config")
		return handleStartSwapError(streamer, err)
	}

	_, err = s.data.GetIntent(ctx, startCurrencyCreatorSwapReq.FundingId)
	if err == nil {
		return handleStartSwapError(streamer, NewSwapValidationError("funding intent already exists"))
	} else if err != intent.ErrIntentNotFound {
		log.WithError(err).Warn("failure getting funding intent record")
		return handleStartSwapError(streamer, err)
	}

	//
	// Section: Server parameters
	//

	noncePool, err := transaction_util.SelectNoncePool(
		nonce.EnvironmentSolana,
		nonce.EnvironmentInstanceSolanaMainnet,
		nonce.PurposeClientSwap,
	)
	if err != nil {
		log.WithError(err).Warn("failure selecting nonce pool")
		return handleStartSwapError(streamer, err)
	}
	selectedNonce, err := noncePool.GetNonce(ctx)
	if err != nil {
		log.WithError(err).Warn("failure selecting available nonce")
		return handleStartSwapError(streamer, err)
	}
	defer func() {
		selectedNonce.ReleaseIfNotReserved(ctx)
	}()

	serverParameters := &transactionpb.StartSwapResponse_ServerParameters_CurrencyCreator{
		Nonce:     selectedNonce.Account.ToProto(),
		Blockhash: &commonpb.Blockhash{Value: selectedNonce.Blockhash[:]},
	}
	if err := streamer.Send(&transactionpb.StartSwapResponse{
		Response: &transactionpb.StartSwapResponse_ServerParameters_{
			ServerParameters: &transactionpb.StartSwapResponse_ServerParameters{
				Kind: &transactionpb.StartSwapResponse_ServerParameters_CurrencyCreator_{
					CurrencyCreator: serverParameters,
				},
			},
		},
	}); err != nil {
		return handleStartSwapError(streamer, err)
	}

	req, err = protoutil.BoundedReceive[transactionpb.StartSwapRequest](ctx, streamer, s.conf.clientReceiveTimeout.Get(ctx))
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return handleStartSwapError(streamer, err)
	}

	//
	// Section: Verified metadata signing
	//

	submitSignatureReq := req.GetSubmitSignature()
	if submitSignatureReq == nil {
		return handleStartSwapError(streamer, status.Error(codes.InvalidArgument, "StartSwapRequest.SubmitSignature is nil"))
	}

	verifiedMetadata := &transactionpb.VerifiedCurrencyCreatorSwapMetadata{
		ClientParameters: startCurrencyCreatorSwapReq,
		ServerParameters: serverParameters,
	}

	metadataSignature := submitSignatureReq.Signature
	if err := s.auth.Authenticate(ctx, owner, verifiedMetadata, metadataSignature); err != nil {
		return handleStartSwapStructuredError(streamer, transactionpb.StartSwapResponse_Error_SIGNATURE_ERROR)
	}

	//
	// Section: Swap state DB commit
	//

	record := &swap.Record{
		SwapId:               swapId,
		Owner:                owner.PublicKey().ToBase58(),
		FromMint:             fromMint.PublicKey().ToBase58(),
		ToMint:               toMint.PublicKey().ToBase58(),
		Amount:               startCurrencyCreatorSwapReq.Amount,
		FundingSource:        swap.FundingSource(startCurrencyCreatorSwapReq.FundingSource),
		FundingId:            startCurrencyCreatorSwapReq.FundingId,
		Nonce:                selectedNonce.Account.PublicKey().ToBase58(),
		Blockhash:            base58.Encode(selectedNonce.Blockhash[:]),
		ProofSignature:       base58.Encode(submitSignatureReq.Signature.Value),
		TransactionSignature: nil,
		TransactionBlob:      nil,
		State:                swap.StateCreated,
		CreatedAt:            time.Now(),
	}

	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err = selectedNonce.MarkReservedWithSignature(ctx, record.ProofSignature)
		if err != nil {
			log.WithError(err).Warn("failure reserving nonce")
			return err
		}

		err = s.data.SaveSwap(ctx, record)
		if err != nil {
			log.WithError(err).Warn("failure saving swap record")
			return err
		}

		return nil
	})
	if err != nil {
		return handleStartSwapError(streamer, err)
	}

	//
	// Section: Final RPC response
	//

	err = streamer.Send(&transactionpb.StartSwapResponse{
		Response: &transactionpb.StartSwapResponse_Success_{
			Success: &transactionpb.StartSwapResponse_Success{
				Code: transactionpb.StartSwapResponse_Success_OK,
			},
		},
	})
	return handleStartSwapError(streamer, err)
}

func (s *transactionServer) GetSwap(ctx context.Context, req *transactionpb.GetSwapRequest) (*transactionpb.GetSwapResponse, error) {
	log := s.log.WithField("method", "GetSwap")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	swapId := base58.Encode(req.Id.Value)
	log = log.WithField("swap_id", swapId)

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	record, err := s.data.GetSwapById(ctx, swapId)
	if err == swap.ErrNotFound {
		return &transactionpb.GetSwapResponse{
			Result: transactionpb.GetSwapResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting swap")
		return nil, status.Error(codes.Internal, "")
	}

	if record.Owner != owner.PublicKey().ToBase58() {
		return &transactionpb.GetSwapResponse{
			Result: transactionpb.GetSwapResponse_DENIED,
		}, nil
	}

	protoSwap, err := toProtoSwap(record)
	if err != nil {
		log.WithError(err).Warn("failure converting swap to proto")
		return nil, status.Error(codes.Internal, "")
	}

	return &transactionpb.GetSwapResponse{
		Result: transactionpb.GetSwapResponse_OK,
		Swap:   protoSwap,
	}, nil
}

func (s *transactionServer) GetPendingSwaps(ctx context.Context, req *transactionpb.GetPendingSwapsRequest) (*transactionpb.GetPendingSwapsResponse, error) {
	log := s.log.WithField("method", "GetPendingSwaps")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	if s.conf.disableSwaps.Get(ctx) {
		return &transactionpb.GetPendingSwapsResponse{
			Result: transactionpb.GetPendingSwapsResponse_OK,
		}, nil
	}

	createdSwaps, err := s.data.GetAllSwapsByOwnerAndState(ctx, owner.PublicKey().ToBase58(), swap.StateCreated)
	if err != nil && err != swap.ErrNotFound {
		log.WithError(err).Warn("failure getting swaps in CREATED state")
		return nil, status.Error(codes.Internal, "")
	}

	fundedSwaps, err := s.data.GetAllSwapsByOwnerAndState(ctx, owner.PublicKey().ToBase58(), swap.StateFunded)
	if err != nil && err != swap.ErrNotFound {
		log.WithError(err).Warn("failure getting swaps in FUNDED state")
		return nil, status.Error(codes.Internal, "")
	}

	allPendingSwaps := createdSwaps
	allPendingSwaps = append(allPendingSwaps, fundedSwaps...)

	if len(allPendingSwaps) == 0 {
		return &transactionpb.GetPendingSwapsResponse{
			Result: transactionpb.GetPendingSwapsResponse_NOT_FOUND,
		}, nil
	}

	res := make([]*transactionpb.SwapMetadata, len(allPendingSwaps))
	for i, pendingSwap := range allPendingSwaps {
		log := log.WithField("swap_id", pendingSwap.SwapId)

		res[i], err = toProtoSwap(pendingSwap)
		if err != nil {
			log.WithError(err).Warn("failure converting swap to proto")
			return nil, status.Error(codes.Internal, "")
		}
	}

	return &transactionpb.GetPendingSwapsResponse{
		Result: transactionpb.GetPendingSwapsResponse_OK,
		Swaps:  res,
	}, nil
}

func (s *transactionServer) Swap(streamer transactionpb.Transaction_SwapServer) error {
	// Bound the total RPC. Keeping the timeout higher to see where we land because
	// there's a lot of stuff happening in this method.
	ctx, cancel := context.WithTimeout(streamer.Context(), s.conf.swapTimeout.Get(streamer.Context()))
	defer cancel()

	log := s.log.WithField("method", "Swap")
	log = log.WithContext(ctx)
	log = client.InjectLoggingMetadata(ctx, log)

	if s.conf.disableSwaps.Get(ctx) {
		return handleSwapError(streamer, status.Error(codes.Unavailable, "temporarily unavailable"))
	}

	req, err := protoutil.BoundedReceive[transactionpb.SwapRequest](ctx, streamer, s.conf.clientReceiveTimeout.Get(ctx))
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return handleSwapError(streamer, err)
	}

	initiateReq := req.GetInitiate()
	if initiateReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate is nil"))
	}

	if initiateReq.GetStateless() != nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate.Stateless is not currently supporteds"))
	}

	statefulReq := initiateReq.GetStateful()
	if statefulReq == nil {
		return handleSwapError(streamer, status.Error(codes.InvalidArgument, "SwapRequest.Initiate.Stateful is nil"))
	}

	owner, err := common.NewAccountFromProto(statefulReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("owner", owner.PublicKey().ToBase58())

	reqSignature := statefulReq.Signature
	statefulReq.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, statefulReq, reqSignature); err != nil {
		return err
	}

	swapId := base58.Encode(statefulReq.SwapId.Value)
	log = log.WithField("swap_id", swapId)

	swapRecord, err := s.data.GetSwapById(ctx, swapId)
	if err == swap.ErrNotFound {
		return handleSwapError(streamer, NewSwapValidationError("swap state not found"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting swap record")
		return handleSwapError(streamer, err)
	}

	swapAuthority, err := common.NewAccountFromProto(statefulReq.SwapAuthority)
	if err != nil {
		log.WithError(err).Warn("invalid swap authority")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("swap_authority", swapAuthority.PublicKey().ToBase58())

	fromMint, err := common.NewAccountFromPublicKeyString(swapRecord.FromMint)
	if err != nil {
		log.WithError(err).Warn("invalid mint")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("from_mint", fromMint.PublicKey().ToBase58())

	toMint, err := common.NewAccountFromPublicKeyString(swapRecord.ToMint)
	if err != nil {
		log.WithError(err).Warn("invalid mint")
		return handleSwapError(streamer, err)
	}
	log = log.WithField("to_mint", toMint.PublicKey().ToBase58())

	nonce, err := common.NewAccountFromPublicKeyString(swapRecord.Nonce)
	if err != nil {
		log.WithError(err).Warn("invalid nonce")
		return handleSwapError(streamer, err)
	}

	decodedBlockhash, err := base58.Decode(swapRecord.Blockhash)
	if err != nil {
		log.WithError(err).Warn("invalid blockhash")
		return handleSwapError(streamer, err)
	}
	blockhash := solana.Blockhash(decodedBlockhash)

	sourceVmConfig, err := common.GetVmConfigForMint(ctx, s.data, fromMint)
	if err != nil {
		log.WithError(err).Warn("failure getting source vm config")
		return handleSwapError(streamer, err)
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, s.data, toMint)
	if err != nil {
		log.WithError(err).Warn("failure getting destination vm config")
		return handleSwapError(streamer, err)
	}

	ownerSourceVmSwapAta, err := owner.ToVmSwapAta(sourceVmConfig)
	if err != nil {
		log.WithError(err).Warn("failure getting owner source vm swap ata")
		return handleSwapError(streamer, err)
	}
	ownerDestinationTimelockVault, err := owner.ToTimelockVault(destinationVmConfig)
	if err != nil {
		log.WithError(err).Warn("failure getting owner destination timelock vault")
		return handleSwapError(streamer, err)
	}

	//
	// Section: Validation
	//

	if swapRecord.Owner != owner.PublicKey().ToBase58() {
		return handleSwapError(streamer, NewSwapDeniedError("not the owner of this swap"))
	}

	if swapRecord.State != swap.StateFunded {
		return handleSwapError(streamer, NewSwapValidationError("swap is not in a funded state"))
	}

	if owner.PublicKey().ToBase58() == swapAuthority.PublicKey().ToBase58() {
		return handleSwapError(streamer, NewSwapValidationError("owner cannot be swap authority"))
	}

	if bytes.Equal(fromMint.PublicKey().ToBytes(), toMint.PublicKey().ToBytes()) {
		return handleSwapError(streamer, NewSwapValidationError("must swap between two different mints"))
	}

	_, err = s.data.GetTimelockByVault(ctx, ownerDestinationTimelockVault.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		return handleSwapError(streamer, NewSwapValidationError("destination timelock vault account not opened"))
	} else if err != nil {
		log.WithError(err).Warn("failure getting destination timelock record")
		return handleSwapError(streamer, err)
	}

	// todo: for any of these invalid funding cases, we should cancel the swap
	intentRecord, err := s.data.GetIntent(ctx, swapRecord.FundingId)
	if err != nil {
		log.WithError(err).Warn("failure getting funding intent record")
		return handleSwapError(streamer, errors.New("unexpected nonce state"))
	}
	if intentRecord.IntentType != intent.SendPublicPayment {
		return handleSwapError(streamer, NewSwapValidationError("funding intent is invalid"))
	}
	if intentRecord.SendPublicPaymentMetadata.Quantity < swapRecord.Amount {
		return handleSwapError(streamer, NewSwapValidationError("funding intent is invalid"))
	}
	if intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount != ownerSourceVmSwapAta.PublicKey().ToBase58() {
		return handleSwapError(streamer, NewSwapValidationError("funding intent is invalid"))
	}

	//
	// Section: On-demand account creation
	//

	err = common.EnsureVirtualTimelockAccountIsInitialized(ctx, s.data, s.vmIndexerClient, destinationVmConfig.Vm, owner, true)
	if err != nil {
		log.WithError(err).Warn("timed out waiting for destination timelock account initialization")
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
			swapRecord.Amount,
			nonce,
		)
	} else if common.IsCoreMint(toMint) {
		swapHandler = NewCurrencyCreatorSellSwapHandler(
			s.data,
			s.vmIndexerClient,
			owner,
			swapAuthority,
			fromMint,
			swapRecord.Amount,
			nonce,
		)
	} else {
		swapHandler = NewCurrencyCreatorBuySellSwapHandler(
			s.data,
			s.vmIndexerClient,
			owner,
			swapAuthority,
			fromMint,
			toMint,
			swapRecord.Amount,
			nonce,
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

	txn.SetBlockhash(solana.Blockhash(blockhash))

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
				Kind: &transactionpb.SwapResponse_ServerParameters_CurrencyCreatorStateful_{
					CurrencyCreatorStateful: &transactionpb.SwapResponse_ServerParameters_CurrencyCreatorStateful{
						Payer:            common.GetSubsidizer().ToProto(),
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

	req, err = protoutil.BoundedReceive[transactionpb.SwapRequest](ctx, streamer, s.conf.clientReceiveTimeout.Get(ctx))
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

	//
	// Section: Swap state transition
	//

	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := transaction_util.UpdateNonceSignature(
			ctx,
			s.data,
			nonce.PublicKey().ToBase58(),
			swapRecord.ProofSignature,
			txnSignature,
		)
		if err != nil {
			log.WithError(err).Warn("failure updating nonce record")
			return err
		}

		swapRecord.State = swap.StateSubmitting
		swapRecord.TransactionSignature = &txnSignature
		swapRecord.TransactionBlob = marshalledTxn
		err = s.data.SaveSwap(ctx, swapRecord)
		if err != nil {
			log.WithError(err).Warn("failure updating swap record")
			return err
		}

		return nil
	})
	if err != nil {
		return handleSwapError(streamer, err)
	}

	//
	// Section: Final RPC response
	//

	err = streamer.Send(&transactionpb.SwapResponse{
		Response: &transactionpb.SwapResponse_Success_{
			Success: &transactionpb.SwapResponse_Success{
				Code: transactionpb.SwapResponse_Success_SWAP_SUBMITTED,
			},
		},
	})
	return handleSwapError(streamer, err)
}

func toProtoSwap(record *swap.Record) (*transactionpb.SwapMetadata, error) {
	decodedSwapId, err := base58.Decode(record.SwapId)
	if err != nil {
		return nil, err
	}

	fromMint, err := common.NewAccountFromPublicKeyString(record.FromMint)
	if err != nil {
		return nil, err
	}

	toMint, err := common.NewAccountFromPublicKeyString(record.ToMint)
	if err != nil {
		return nil, err
	}

	nonce, err := common.NewAccountFromPublicKeyString(record.Nonce)
	if err != nil {
		return nil, err
	}

	decodedBlockhash, err := base58.Decode(record.Blockhash)
	if err != nil {
		return nil, err
	}

	decodedSignature, err := base58.Decode(record.ProofSignature)
	if err != nil {
		return nil, err
	}

	return &transactionpb.SwapMetadata{
		VerifiedMetadata: &transactionpb.VerifiedSwapMetadata{
			Kind: &transactionpb.VerifiedSwapMetadata_CurrencyCreator{
				CurrencyCreator: &transactionpb.VerifiedCurrencyCreatorSwapMetadata{
					ClientParameters: &transactionpb.StartSwapRequest_Start_CurrencyCreator{
						Id:            &commonpb.SwapId{Value: decodedSwapId},
						FromMint:      fromMint.ToProto(),
						ToMint:        toMint.ToProto(),
						Amount:        record.Amount,
						FundingSource: transactionpb.FundingSource(record.FundingSource),
						FundingId:     record.FundingId,
					},
					ServerParameters: &transactionpb.StartSwapResponse_ServerParameters_CurrencyCreator{
						Nonce:     nonce.ToProto(),
						Blockhash: &commonpb.Blockhash{Value: decodedBlockhash},
					},
				},
			},
		},
		State:     transactionpb.SwapMetadata_State(record.State),
		Signature: &commonpb.Signature{Value: decodedSignature},
	}, nil
}
