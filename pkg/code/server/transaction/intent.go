package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	async_account "github.com/code-payments/code-server/pkg/code/async/account"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/transaction"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/token"
)

func (s *transactionServer) SubmitIntent(streamer transactionpb.Transaction_SubmitIntentServer) error {
	// Bound the total RPC. Keeping the timeout higher to see where we land because
	// there's a lot of stuff happening in this method.
	ctx, cancel := context.WithTimeout(streamer.Context(), s.conf.submitIntentTimeout.Get(streamer.Context()))
	defer cancel()

	log := s.log.WithField("method", "SubmitIntent")
	log = log.WithContext(ctx)
	log = client.InjectLoggingMetadata(ctx, log)

	if s.conf.disableSubmitIntent.Get(ctx) {
		return status.Error(codes.Unavailable, "temporarily unavailable")
	}

	okResp := &transactionpb.SubmitIntentResponse{
		Response: &transactionpb.SubmitIntentResponse_Success_{
			Success: &transactionpb.SubmitIntentResponse_Success{
				Code: transactionpb.SubmitIntentResponse_Success_OK,
			},
		},
	}

	// Client initiates phase 1 of the RPC by submitting and intent via a set of
	// actions and metadata.
	req, err := s.boundedSubmitIntentRecv(ctx, streamer)
	if err != nil {
		log.WithError(err).Info("error receiving request from client")
		return handleSubmitIntentError(streamer, err)
	}

	start := time.Now()

	submitActionsReq := req.GetSubmitActions()
	if submitActionsReq == nil {
		return status.Error(codes.InvalidArgument, "SubmitIntentRequest.SubmitActions is nil")
	}

	// Do some very basic and general validation that cannot be caught by
	// proto validation
	for i, protoAction := range submitActionsReq.Actions {
		if protoAction.Id != uint32(i) {
			return handleSubmitIntentError(streamer, status.Errorf(codes.InvalidArgument, "invalid SubmitIntentRequest.SubmitActions.Actions[%d].Id", i))
		}
	}

	intentId := base58.Encode(submitActionsReq.Id.Value)
	log = log.WithField("intent", intentId)

	marshalled, err := proto.Marshal(submitActionsReq)
	if err == nil {
		log = log.WithField("submit_actions_data_dump", base64.URLEncoding.EncodeToString(marshalled))
	}

	// Figure out what kind of intent we're operating on and initialize the intent handler
	var intentHandler CreateIntentHandler
	switch submitActionsReq.Metadata.Type.(type) {
	case *transactionpb.Metadata_OpenAccounts:
		log = log.WithField("intent_type", "open_accounts")
		intentHandler = NewOpenAccountsIntentHandler(s.conf, s.data, s.antispamGuard)
	case *transactionpb.Metadata_SendPublicPayment:
		log = log.WithField("intent_type", "send_public_payment")
		intentHandler = NewSendPublicPaymentIntentHandler(s.conf, s.data, s.antispamGuard, s.amlGuard)
	case *transactionpb.Metadata_ReceivePaymentsPublicly:
		log = log.WithField("intent_type", "receive_payments_publicly")
		intentHandler = NewReceivePaymentsPubliclyIntentHandler(s.conf, s.data, s.antispamGuard, s.amlGuard)
	case *transactionpb.Metadata_PublicDistribution:
		log = log.WithField("intent_type", "public_distribution")
		intentHandler = NewPublicDistributionIntentHandler(s.conf, s.data, s.antispamGuard, s.amlGuard)
	default:
		return handleSubmitIntentError(streamer, status.Error(codes.InvalidArgument, "SubmitIntentRequest.SubmitActions.Metadata is nil"))
	}

	// The public key that is the owner and signed the intent. This may not be
	// the user depending upon the context of how the user initiated the intent.
	submitActionsOwnerAccount, err := common.NewAccountFromProto(submitActionsReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid submit actions owner account")
		return handleSubmitIntentError(streamer, err)
	}
	log = log.WithField("submit_actions_owner_account", submitActionsOwnerAccount.PublicKey().ToBase58())

	createsNewUserOwner, err := intentHandler.CreatesNewUser(ctx, submitActionsReq.Metadata)
	if err != nil {
		log.WithError(err).Warn("failure checking if intent creates a new user")
		return handleSubmitIntentError(streamer, err)
	}

	var initiatorOwnerAccount *common.Account
	submitActionsOwnerMetadata, err := common.GetOwnerMetadata(ctx, s.data, submitActionsOwnerAccount)
	if err == nil {
		switch submitActionsOwnerMetadata.Type {
		case common.OwnerTypeUser12Words:
			initiatorOwnerAccount = submitActionsOwnerAccount
		case common.OwnerTypeRemoteSendGiftCard:
			// Remote send gift cards can only be the owner of an intent for a
			// remote send public receive. In this instance, we need to inspect
			// the destination account, which should be a user's primary
			// account.
			//
			// todo: This is a bit of a mess and should realistically be a generic
			//       method for intent handlers.
			switch typed := submitActionsReq.Metadata.Type.(type) {
			case *transactionpb.Metadata_ReceivePaymentsPublicly:
				if typed.ReceivePaymentsPublicly.IsRemoteSend {
					switch typed := submitActionsReq.Actions[0].Type.(type) {
					case *transactionpb.Action_NoPrivacyWithdraw:
						accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, base58.Encode(typed.NoPrivacyWithdraw.Destination.Value))
						if err != nil && err != account.ErrAccountInfoNotFound {
							log.WithError(err).Warn("failure getting user initiator owner account")
							return handleSubmitIntentError(streamer, err)
						} else if err == account.ErrAccountInfoNotFound || accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
							return handleSubmitIntentError(streamer, NewActionValidationError(submitActionsReq.Actions[0], "destination must be a primary account"))
						}

						initiatorOwnerAccount, err = common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
						if err != nil {
							log.WithError(err).Warn("failure getting user initiator owner account")
							return handleSubmitIntentError(streamer, err)
						}
					default:
						return NewActionValidationError(submitActionsReq.Actions[0], "expected a no privacy withdraw action")
					}
				}
			default:
				return NewIntentValidationError("expected a receive payments publicly intent")
			}
		default:
			log.Warnf("unhandled owner account type %s", submitActionsOwnerMetadata.Type)
			return handleSubmitIntentError(streamer, errors.New("unhandled owner account type"))
		}
	} else if err == common.ErrOwnerNotFound {
		if !createsNewUserOwner {
			return handleSubmitIntentError(streamer, NewIntentDeniedError("unexpected owner account"))
		}
		initiatorOwnerAccount = submitActionsOwnerAccount
	} else if err != nil {
		log.WithError(err).Warn("failure getting owner account metadata")
		return handleSubmitIntentError(streamer, err)
	}

	log = log.WithField("initiator_owner_account", initiatorOwnerAccount.PublicKey().ToBase58())

	// Check that all provided signatures in proto messages are valid
	signature := submitActionsReq.Signature
	submitActionsReq.Signature = nil
	err = s.auth.Authenticate(ctx, submitActionsOwnerAccount, submitActionsReq, signature)
	if err != nil {
		return handleSubmitIntentError(streamer, err)
	}

	for _, action := range submitActionsReq.Actions {
		switch typedAction := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			authorityAccount, err := common.NewAccountFromProto(typedAction.OpenAccount.Authority)
			if err != nil {
				return handleSubmitIntentError(streamer, err)
			}

			switch typedAction.OpenAccount.AccountType {
			case commonpb.AccountType_REMOTE_SEND_GIFT_CARD:
				// Remote gift cards are random accounts not owned by a user account's 12 words
				if !bytes.Equal(typedAction.OpenAccount.Owner.Value, typedAction.OpenAccount.Authority.Value) {
					return handleSubmitIntentError(streamer, NewActionValidationErrorf(action, "owner must be %s", authorityAccount.PublicKey().ToBase58()))
				}
			default:
				// Everything else is owned by a user account's 12 words
				if !bytes.Equal(typedAction.OpenAccount.Owner.Value, initiatorOwnerAccount.PublicKey().ToBytes()) {
					return handleSubmitIntentError(streamer, NewActionValidationErrorf(action, "owner must be %s", initiatorOwnerAccount.PublicKey().ToBase58()))
				}
			}

			signature := typedAction.OpenAccount.AuthoritySignature
			typedAction.OpenAccount.AuthoritySignature = nil
			err = s.auth.Authenticate(ctx, authorityAccount, typedAction.OpenAccount, signature)
			if err != nil {
				return handleSubmitIntentError(streamer, err)
			}
		}
	}

	intentRecord := &intent.Record{
		IntentId:              intentId,
		InitiatorOwnerAccount: initiatorOwnerAccount.PublicKey().ToBase58(),
		State:                 intent.StatePending,
		CreatedAt:             time.Now(),
	}

	existingIntentRecord, err := s.data.GetIntent(ctx, intentId)
	if err != intent.ErrIntentNotFound && err != nil {
		log.WithError(err).Warn("failure checking for existing intent record")
		return handleSubmitIntentError(streamer, err)
	}

	// We're operating on a new intent, so validate we don't have an existing DB record
	if existingIntentRecord != nil {
		log.Warn("client is attempting to resubmit an intent or reuse an intent id")
		return handleSubmitIntentError(streamer, NewStaleStateError("intent already exists"))
	}

	// Populate metadata into the new DB record
	err = intentHandler.PopulateMetadata(ctx, intentRecord, submitActionsReq.Metadata)
	if err != nil {
		switch err.(type) {
		case IntentValidationError:
			log.WithError(err).Warn("new intent failed validation")
		case IntentDeniedError:
			log.WithError(err).Warn("new intent was denied")
		case StaleStateError:
			log.WithError(err).Warn("detected a client with stale state")
		default:
			log.WithError(err).Warn("failure populating intent metadata")
		}
		return handleSubmitIntentError(streamer, err)
	}

	// Check whether the intent is a no-op
	isNoop, err := intentHandler.IsNoop(ctx, intentRecord, submitActionsReq.Metadata, submitActionsReq.Actions)
	if err != nil {
		log.WithError(err).Warn("failure checking if intent is a no-op")
		return handleSubmitIntentError(streamer, err)
	} else if isNoop {
		if err := streamer.Send(okResp); err != nil {
			return handleSubmitIntentError(streamer, err)
		}
		return nil
	}

	// Lock any acccounts with fund movement that is not resistent to race conditions
	//  1. Global DB layer lock to guarantee balance consistency in a mult-server environment
	//  2. Local in memory lock to avoid over consumption of local resources (eg.
	//     nonces) when we're likely to encounter a race resulting in DB txn rollback
	//     (eg. mass attempt to claim gift card).
	globalBalanceLocks, err := intentHandler.GetBalanceLocks(ctx, intentRecord, submitActionsReq.Metadata)
	if err != nil {
		log.WithError(err).Warn("failure getting accounts with balances to lock")
		return handleSubmitIntentError(streamer, err)
	}
	localAccountLocks := make([]*sync.Mutex, 0)
	locallyLockedAccounts := make(map[string]any)
	for _, globalBalanceLock := range globalBalanceLocks {
		_, ok := locallyLockedAccounts[globalBalanceLock.Account.PublicKey().ToBase58()]
		if !ok {
			localAccountLocks = append(localAccountLocks, s.getLocalAccountLock(globalBalanceLock.Account))
		}
		locallyLockedAccounts[globalBalanceLock.Account.PublicKey().ToBase58()] = true
	}
	for _, localAccountLock := range localAccountLocks {
		localAccountLock.Lock()
		defer localAccountLock.Unlock()
	}

	// Validate the new intent with intent-specific logic
	err = intentHandler.AllowCreation(ctx, intentRecord, submitActionsReq.Metadata, submitActionsReq.Actions)
	if err != nil {
		switch err.(type) {
		case IntentValidationError:
			log.WithError(err).Warn("new intent failed validation")
		default:
			log.WithError(err).Warn("failure checking if new intent was allowed")
		}
		return handleSubmitIntentError(streamer, err)
	}

	// Validate the new intent with app-specific logic
	err = s.submitIntentIntegration.AllowCreation(ctx, intentRecord, submitActionsReq.Metadata, submitActionsReq.Actions)
	if err != nil {
		switch err.(type) {
		case IntentValidationError:
			log.WithError(err).Warn("new intent failed integration validation")
		case IntentDeniedError:
			log.WithError(err).Warn("new intent was denied by integration")
		case StaleStateError:
			log.WithError(err).Warn("integration detected a client with stale state")
		default:
			log.WithError(err).Warn("failure checking if new intent was allowed by integration")
		}
		return handleSubmitIntentError(streamer, err)
	}

	type fulfillmentWithSigningMetadata struct {
		record *fulfillment.Record

		requiresClientSignature bool
		expectedSigner          *common.Account
		virtualIxnHash          *cvm.CompactMessage

		intentOrderingIndexOverriden bool
	}

	// Convert all actions into a set of fulfillments
	var actionHandlers []CreateActionHandler
	var actionRecords []*action.Record
	var fulfillments []fulfillmentWithSigningMetadata
	var reservedNonces []*transaction.Nonce
	var serverParameters []*transactionpb.ServerParameter
	for i, protoAction := range submitActionsReq.Actions {
		log := log.WithField("action_id", i)

		// Figure out what kind of action we're operating on and initialize the
		// action handler
		var actionHandler CreateActionHandler
		var actionType action.Type
		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			log = log.WithField("action_type", "open_account")
			actionType = action.OpenAccount
			actionHandler, err = NewOpenAccountActionHandler(ctx, s.data, typed.OpenAccount, submitActionsReq.Metadata)
		case *transactionpb.Action_NoPrivacyTransfer:
			log = log.WithField("action_type", "no_privacy_transfer")
			actionType = action.NoPrivacyTransfer
			actionHandler, err = NewNoPrivacyTransferActionHandler(ctx, s.data, typed.NoPrivacyTransfer)
		case *transactionpb.Action_FeePayment:
			log = log.WithField("action_type", "fee_payment")
			actionType = action.NoPrivacyTransfer
			actionHandler, err = NewFeePaymentActionHandler(ctx, s.data, typed.FeePayment, s.feeCollector)
		case *transactionpb.Action_NoPrivacyWithdraw:
			log = log.WithField("action_type", "no_privacy_withdraw")
			actionType = action.NoPrivacyWithdraw
			actionHandler, err = NewNoPrivacyWithdrawActionHandler(ctx, s.data, intentRecord, typed.NoPrivacyWithdraw)
		default:
			return handleSubmitIntentError(streamer, status.Errorf(codes.InvalidArgument, "SubmitIntentRequest.SubmitActions.Actions[%d].Type is nil", i))
		}
		if err != nil {
			log.WithError(err).Warn("failure initializing action handler")
			return handleSubmitIntentError(streamer, errors.New("error initializing action handler"))
		}

		actionHandlers = append(actionHandlers, actionHandler)

		// Construct the equivalent action record
		actionRecord := &action.Record{
			Intent:     intentRecord.IntentId,
			IntentType: intentRecord.IntentType,

			ActionId:   protoAction.Id,
			ActionType: actionType,

			State: action.StateUnknown,
		}

		err := actionHandler.PopulateMetadata(actionRecord)
		if err != nil {
			log.WithError(err).Warn("failure populating action metadata")
			return handleSubmitIntentError(streamer, err)
		}

		actionRecords = append(actionRecords, actionRecord)

		// Get action-specific server parameters needed by client to construct the transaction
		serverParameter := actionHandler.GetServerParameter()
		serverParameter.ActionId = protoAction.Id
		serverParameters = append(serverParameters, serverParameter)

		fulfillmentCount := actionHandler.FulfillmentCount()

		for j := range fulfillmentCount {
			var newFulfillmentMetadata *newFulfillmentMetadata
			var actionId uint32

			// Select any available nonce reserved for use for a client transaction,
			// if it's required
			var selectedNonce *transaction.Nonce
			var nonceAccount *common.Account
			var nonceBlockchash solana.Blockhash
			requiresNonce, vmAccount := actionHandler.RequiresNonce(j)
			if requiresNonce {
				noncePool, err := transaction.SelectNoncePool(
					nonce.EnvironmentCvm,
					vmAccount.PublicKey().ToBase58(),
					nonce.PurposeClientTransaction,
					s.noncePools...,
				)
				if err != nil {
					log.WithError(err).Warn("failure selecting nonce pool")
					return handleSubmitIntentError(streamer, err)
				}

				selectedNonce, err = noncePool.GetNonce(ctx)
				if err != nil {
					log.WithError(err).Warn("failure selecting available nonce")
					return handleSubmitIntentError(streamer, err)
				}
				defer func() {
					// If we never assign the nonce a signature in the action creation flow,
					// it's safe to put it back in the available pool. The client will have
					// caused a failed RPC call, and we want to avoid malicious or erroneous
					// clients from consuming our nonce pool!
					selectedNonce.ReleaseIfNotReserved(ctx)
				}()
				nonceAccount = selectedNonce.Account
				nonceBlockchash = selectedNonce.Blockhash
			}

			// Get metadata for the new fulfillment being created
			newFulfillmentMetadata, err = actionHandler.GetFulfillmentMetadata(
				j,
				nonceAccount,
				nonceBlockchash,
			)
			if err != nil {
				log.WithError(err).Warn("failure getting fulfillment metadata")
				return handleSubmitIntentError(streamer, err)
			}

			actionId = protoAction.Id

			// Construct the fulfillment record
			fulfillmentRecord := &fulfillment.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentRecord.IntentType,

				ActionId:   actionId,
				ActionType: actionType,

				FulfillmentType: newFulfillmentMetadata.fulfillmentType,

				Source: newFulfillmentMetadata.source.PublicKey().ToBase58(),

				IntentOrderingIndex:      0, // Unknown until intent record is saved, so it's injected later
				ActionOrderingIndex:      actionId,
				FulfillmentOrderingIndex: newFulfillmentMetadata.fulfillmentOrderingIndex,

				DisableActiveScheduling: newFulfillmentMetadata.disableActiveScheduling,

				State: fulfillment.StateUnknown,
			}
			if newFulfillmentMetadata.destination != nil {
				fulfillmentRecord.Destination = pointer.String(newFulfillmentMetadata.destination.PublicKey().ToBase58())
			}
			if newFulfillmentMetadata.intentOrderingIndexOverride != nil {
				fulfillmentRecord.IntentOrderingIndex = *newFulfillmentMetadata.intentOrderingIndexOverride
			}
			if newFulfillmentMetadata.actionOrderingIndexOverride != nil {
				fulfillmentRecord.ActionOrderingIndex = *newFulfillmentMetadata.actionOrderingIndexOverride
			}

			// Fulfillment has a virtual instruction requiring client signature
			if newFulfillmentMetadata.requiresClientSignature {
				fulfillmentRecord.VirtualNonce = pointer.String(selectedNonce.Account.PublicKey().ToBase58())
				fulfillmentRecord.VirtualBlockhash = pointer.String(base58.Encode(selectedNonce.Blockhash[:]))

				serverParameter.Nonces = append(serverParameter.Nonces, &transactionpb.NoncedTransactionMetadata{
					Nonce: selectedNonce.Account.ToProto(),
					Blockhash: &commonpb.Blockhash{
						Value: selectedNonce.Blockhash[:],
					},
				})
			}

			fulfillments = append(fulfillments, fulfillmentWithSigningMetadata{
				record: fulfillmentRecord,

				requiresClientSignature: newFulfillmentMetadata.requiresClientSignature,
				expectedSigner:          newFulfillmentMetadata.expectedSigner,
				virtualIxnHash:          newFulfillmentMetadata.virtualIxnHash,

				intentOrderingIndexOverriden: newFulfillmentMetadata.intentOrderingIndexOverride != nil,
			})
			reservedNonces = append(reservedNonces, selectedNonce)
		}
	}

	serverParametersResp := &transactionpb.SubmitIntentResponse{
		Response: &transactionpb.SubmitIntentResponse_ServerParameters_{
			ServerParameters: &transactionpb.SubmitIntentResponse_ServerParameters{
				ServerParameters: serverParameters,
			},
		},
	}

	var unsignedFulfillments []fulfillmentWithSigningMetadata
	for _, fulfillmentWithMetadata := range fulfillments {
		if fulfillmentWithMetadata.requiresClientSignature {
			unsignedFulfillments = append(unsignedFulfillments, fulfillmentWithMetadata)
		}
	}

	metricsIntentTypeValue := intentRecord.IntentType.String()
	latencyBeforeSignatureSubmission := time.Since(start)
	recordSubmitIntentLatencyBreakdownEvent(
		ctx,
		"BeforeSignatureSubmission",
		latencyBeforeSignatureSubmission,
		len(submitActionsReq.Actions),
		metricsIntentTypeValue,
	)
	tsAfterSignatureSubmission := time.Now()

	// Process fulfillments that require client signatures, if there are any
	if len(unsignedFulfillments) > 0 {
		// Send server parameters, which initiates phase 2 of the RPC for generating
		// and receiving signatures.
		if err := streamer.Send(serverParametersResp); err != nil {
			return handleSubmitIntentError(streamer, err)
		}

		req, err = s.boundedSubmitIntentRecv(ctx, streamer)
		if err != nil {
			log.WithError(err).Info("error receiving request from client")
			return handleSubmitIntentError(streamer, err)
		}

		tsAfterSignatureSubmission = time.Now()

		submitSignaturesReq := req.GetSubmitSignatures()
		if submitSignaturesReq == nil {
			return handleSubmitIntentError(streamer, status.Error(codes.InvalidArgument, "SubmitIntentRequest.SubmitSignatures is nil"))
		}

		marshalled, err := proto.Marshal(submitSignaturesReq)
		if err == nil {
			log = log.WithField("submit_signatures_data_dump", base64.URLEncoding.EncodeToString(marshalled))
		}

		// Validate the number of signatures
		if len(submitSignaturesReq.Signatures) < len(unsignedFulfillments) {
			return handleSubmitIntentError(streamer, ErrMissingSignature)
		} else if len(submitSignaturesReq.Signatures) > len(unsignedFulfillments) {
			return handleSubmitIntentError(streamer, ErrTooManySignatures)
		}

		// Validate the signature value and update the fulfillment
		var signatureErrorDetails []*transactionpb.ErrorDetails
		for i, signature := range submitSignaturesReq.Signatures {
			unsignedFulfillment := unsignedFulfillments[i]

			if !ed25519.Verify(
				unsignedFulfillment.expectedSigner.PublicKey().ToBytes(),
				unsignedFulfillment.virtualIxnHash[:],
				signature.Value,
			) {
				signatureErrorDetails = append(signatureErrorDetails, toInvalidVirtualIxnSignatureErrorDetails(unsignedFulfillment.record.ActionId, *unsignedFulfillment.virtualIxnHash, signature))
			}

			unsignedFulfillment.record.VirtualSignature = pointer.String(base58.Encode(signature.Value))
		}

		if len(signatureErrorDetails) > 0 {
			return handleSubmitIntentStructuredError(
				streamer,
				transactionpb.SubmitIntentResponse_Error_SIGNATURE_ERROR,
				signatureErrorDetails...,
			)
		}
	}

	// Save all of the required DB records in one transaction to complete the
	// intent operation. It's very bad if we end up failing halfway through.
	//
	// Note: This is the first use case of this new method to do this kind of
	// operation. Not all store implementations have real support for this, so
	// if anything is added, then ensure it does!
	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		// Save the intent record
		err = s.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure saving intent record")
			return err
		}

		// Save all actions
		err = s.data.PutAllActions(ctx, actionRecords...)
		if err != nil {
			log.WithError(err).Warn("failure saving action records")
			return err
		}

		// Save additional state related to each action
		for _, actionHandler := range actionHandlers {
			err = actionHandler.OnCommitToDB(ctx)
			if err != nil {
				log.WithError(err).Warn("failure executing action db save callback handler")
				return err
			}
		}

		// Save all fulfillment records
		fulfillmentRecordsToSave := make([]*fulfillment.Record, 0)
		for i, fulfillmentWithMetadata := range fulfillments {
			if !fulfillmentWithMetadata.intentOrderingIndexOverriden {
				fulfillmentWithMetadata.record.IntentOrderingIndex = intentRecord.Id
			}

			fulfillmentRecordsToSave = append(fulfillmentRecordsToSave, fulfillmentWithMetadata.record)

			// Reserve the nonce with the latest server-signed fulfillment.
			if fulfillmentWithMetadata.requiresClientSignature {
				nonceToReserve := reservedNonces[i]

				err = nonceToReserve.MarkReservedWithSignature(ctx, *fulfillmentWithMetadata.record.VirtualSignature)
				if err != nil {
					log.WithError(err).Warn("failure reserving nonce with fulfillment signature")
					return err
				}
			}
		}
		err = s.data.PutAllFulfillments(ctx, fulfillmentRecordsToSave...)
		if err != nil {
			log.WithError(err).Warn("failure saving fulfillment records")
			return err
		}

		for _, globalBalanceLock := range globalBalanceLocks {
			err = globalBalanceLock.CommitFn(ctx, s.data)
			if err != nil {
				log.WithError(err).Warn("failure commiting balance update")
				return err
			}
		}

		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "stale") || strings.Contains(err.Error(), "exist") {
			log.WithError(err).Info("race condition detected")
			return handleSubmitIntentError(streamer, NewStaleStateErrorf("race detected: %s", err.Error()))
		}
		return handleSubmitIntentError(streamer, err)
	}

	go func() {
		err := s.submitIntentIntegration.OnSuccess(context.Background(), intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure calling integration success callback")
		}
	}()

	//
	// Intent is submitted, and anything beyond this point is best-effort.
	// We must send success back to the client. Rolling back the intent is
	// not an option here, since it's already being processed by workers.
	//

	log.Debug("intent submitted")

	// Fire off some success metrics
	recordUserIntentCreatedEvent(ctx, intentRecord)

	latencyAfterSignatureSubmission := time.Since(tsAfterSignatureSubmission)
	recordSubmitIntentLatencyBreakdownEvent(
		ctx,
		"AfterSignatureSubmission",
		latencyAfterSignatureSubmission,
		len(submitActionsReq.Actions),
		metricsIntentTypeValue,
	)
	recordSubmitIntentLatencyBreakdownEvent(
		ctx,
		"Total",
		latencyBeforeSignatureSubmission+latencyAfterSignatureSubmission,
		len(submitActionsReq.Actions),
		metricsIntentTypeValue,
	)

	// RPC is finished. Send success to the client
	if err := streamer.Send(okResp); err != nil {
		return handleSubmitIntentError(streamer, err)
	}
	return nil
}

func (s *transactionServer) boundedSubmitIntentRecv(ctx context.Context, streamer transactionpb.Transaction_SubmitIntentServer) (req *transactionpb.SubmitIntentRequest, err error) {
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

func (s *transactionServer) GetIntentMetadata(ctx context.Context, req *transactionpb.GetIntentMetadataRequest) (*transactionpb.GetIntentMetadataResponse, error) {
	intentId := base58.Encode(req.IntentId.Value)

	log := s.log.WithFields(logrus.Fields{
		"method": "GetIntentMetadata",
		"intent": intentId,
	})
	client.InjectLoggingMetadata(ctx, log)

	var signer *common.Account
	var err error
	if req.Owner != nil {
		signer, err = common.NewAccountFromProto(req.Owner)
		if err != nil {
			log.WithError(err).Warn("invalid owner account")
			return nil, status.Error(codes.Internal, "")
		}
	} else {
		signer, err = common.NewAccountFromPublicKeyString(intentId)
		if err != nil {
			log.WithError(err).Warn("invalid intent id")
			return nil, status.Error(codes.Internal, "")
		}
	}
	log = log.WithField("signer", signer.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, signer, req, signature); err != nil {
		return nil, err
	}

	intentRecord, err := s.data.GetIntent(ctx, intentId)
	if err == intent.ErrIntentNotFound {
		return &transactionpb.GetIntentMetadataResponse{
			Result: transactionpb.GetIntentMetadataResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting intent record")
		return nil, status.Error(codes.Internal, "")
	}

	log = log.WithField("intent_type", intentRecord.IntentType.String())

	var destinationOwnerAccount string
	switch intentRecord.IntentType {
	case intent.SendPublicPayment:
		destinationOwnerAccount = intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount
	}

	if req.Owner != nil {
		if signer.PublicKey().ToBase58() != intentRecord.InitiatorOwnerAccount && signer.PublicKey().ToBase58() != destinationOwnerAccount {
			// Owner is not involved in this intent. Don't reveal anything.
			return &transactionpb.GetIntentMetadataResponse{
				Result: transactionpb.GetIntentMetadataResponse_NOT_FOUND,
			}, nil
		}
	}

	var metadata *transactionpb.Metadata
	switch intentRecord.IntentType {
	case intent.SendPublicPayment:
		mintAccount, err := common.NewAccountFromPublicKeyString(intentRecord.MintAccount)
		if err != nil {
			log.WithError(err).Warn("invalid mint account")
			return nil, status.Error(codes.Internal, "")
		}

		sourceAccountInfoRecordsByMint, err := s.data.GetAccountInfoByAuthorityAddress(ctx, intentRecord.InitiatorOwnerAccount)
		if err != nil {
			log.WithError(err).Warn("failure getting source account info record")
			return nil, status.Error(codes.Internal, "")
		}
		sourceAccountInfoRecord, ok := sourceAccountInfoRecordsByMint[mintAccount.PublicKey().ToBase58()]
		if !ok {
			log.WithError(err).Warn("core mint source account info record doesn't exist")
			return nil, status.Error(codes.Internal, "")
		}

		sourceAccount, err := common.NewAccountFromPublicKeyString(sourceAccountInfoRecord.TokenAccount)
		if err != nil {
			log.WithError(err).Warn("invalid source account")
			return nil, status.Error(codes.Internal, "")
		}

		destinationAccount, err := common.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
		if err != nil {
			log.WithError(err).Warn("invalid destination account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_SendPublicPayment{
				SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
					Source:      sourceAccount.ToProto(),
					Destination: destinationAccount.ToProto(),
					ExchangeData: &transactionpb.ExchangeData{
						Currency:     strings.ToLower(string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency)),
						ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
						NativeAmount: intentRecord.SendPublicPaymentMetadata.NativeAmount,
						Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
						Mint:         mintAccount.ToProto(),
					},
					IsRemoteSend: intentRecord.SendPublicPaymentMetadata.IsRemoteSend,
					IsWithdrawal: intentRecord.SendPublicPaymentMetadata.IsWithdrawal,
					Mint:         mintAccount.ToProto(),
				},
			},
		}
	case intent.ReceivePaymentsPublicly:
		mintAccount, err := common.NewAccountFromPublicKeyString(intentRecord.MintAccount)
		if err != nil {
			log.WithError(err).Warn("invalid mint account")
			return nil, status.Error(codes.Internal, "")
		}

		sourceAccount, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
		if err != nil {
			log.WithError(err).Warn("invalid source account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_ReceivePaymentsPublicly{
				ReceivePaymentsPublicly: &transactionpb.ReceivePaymentsPubliclyMetadata{
					Source:       sourceAccount.ToProto(),
					Quarks:       intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
					IsRemoteSend: intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend,
					ExchangeData: &transactionpb.ExchangeData{
						Currency:     string(intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency),
						ExchangeRate: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate,
						NativeAmount: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount,
						Quarks:       intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
						Mint:         mintAccount.ToProto(),
					},
					Mint: mintAccount.ToProto(),
				},
			},
		}
	default:
		// Don't reveal anything for these intent types
		return &transactionpb.GetIntentMetadataResponse{
			Result: transactionpb.GetIntentMetadataResponse_NOT_FOUND,
		}, nil
	}

	return &transactionpb.GetIntentMetadataResponse{
		Result:   transactionpb.GetIntentMetadataResponse_OK,
		Metadata: metadata,
	}, nil
}

func (s *transactionServer) CanWithdrawToAccount(ctx context.Context, req *transactionpb.CanWithdrawToAccountRequest) (*transactionpb.CanWithdrawToAccountResponse, error) {
	log := s.log.WithField("method", "CanWithdrawToAccount")
	log = client.InjectLoggingMetadata(ctx, log)

	accountToCheck, err := common.NewAccountFromProto(req.Account)
	if err != nil {
		log.WithError(err).Warn("invalid account provided")
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	}
	log = log.WithField("account", accountToCheck.PublicKey().ToBase58())

	isOnCurve := accountToCheck.IsOnCurve()

	//
	// Part 1: Is this a Timelock vault? If so, only allow primary accounts.
	//

	if !isOnCurve {
		accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, accountToCheck.PublicKey().ToBase58())
		switch err {
		case nil:
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: accountInfoRecord.AccountType == commonpb.AccountType_PRIMARY,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			}, nil
		case account.ErrAccountInfoNotFound:
			// Nothing to do
		default:
			log.WithError(err).Warn("failure checking account info db")
			return nil, status.Error(codes.Internal, "")
		}
	}

	//
	// Part 2: Is this an opened core mint token account? If so, allow it.
	//

	if !isOnCurve {
		_, err = s.data.GetBlockchainTokenAccountInfo(ctx, accountToCheck.PublicKey().ToBase58(), solana.CommitmentFinalized)
		switch err {
		case nil:
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: true,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			}, nil
		case token.ErrAccountNotFound, solana.ErrNoAccountInfo, token.ErrInvalidTokenAccount:
			// Nothing to do
		default:
			log.WithError(err).Warn("failure checking against blockchain as a token account")
			return nil, status.Error(codes.Internal, "")
		}
	}

	//
	// Part 3: Is this an owner account with an opened Core Mint ATA? If so, allow it.
	//         If not, indicate to the client to pay a fee for a create-on-send withdrawal.
	//

	var isVmDepositPda bool
	_, err = s.data.GetTimelockByDepositPda(ctx, accountToCheck.PublicKey().ToBase58())
	switch err {
	case nil:
		isVmDepositPda = true
	case timelock.ErrTimelockNotFound:
	default:
		log.WithError(err).Warn("failure checking timelock db as a deposit pda account")
		return nil, status.Error(codes.Internal, "")
	}

	if !isOnCurve && !isVmDepositPda {
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	}

	ata, err := accountToCheck.ToAssociatedTokenAccount(common.CoreMintAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting ata address")
		return nil, status.Error(codes.Internal, "")
	}

	_, err = s.data.GetBlockchainTokenAccountInfo(ctx, ata.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: true,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_OwnerAccount,
		}, nil
	case token.ErrAccountNotFound, solana.ErrNoAccountInfo:
		// ATA doesn't exist, and we won't be subsidizing it. Let the client know
		// they require a fee.
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: true,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_OwnerAccount,
			RequiresInitialization:    true,
			FeeAmount: &transactionpb.ExchangeDataWithoutRate{
				Currency:     string(currency_lib.USD),
				NativeAmount: s.conf.createOnSendWithdrawalUsdFee.Get(ctx),
			},
		}, nil
	case token.ErrInvalidTokenAccount:
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: false,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		}, nil
	default:
		log.WithError(err).Warn("failure checking against blockchain as an owner account")
		return nil, status.Error(codes.Internal, "")
	}
}

func (s *transactionServer) VoidGiftCard(ctx context.Context, req *transactionpb.VoidGiftCardRequest) (*transactionpb.VoidGiftCardResponse, error) {
	log := s.log.WithField("method", "VoidGiftCard")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	giftCardVault, err := common.NewAccountFromProto(req.GiftCardVault)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("gift_card_vault_account", giftCardVault.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, giftCardVault.PublicKey().ToBase58())
	switch err {
	case nil:
		if accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
			return &transactionpb.VoidGiftCardResponse{
				Result: transactionpb.VoidGiftCardResponse_NOT_FOUND,
			}, nil
		}
	case account.ErrAccountInfoNotFound:
		return &transactionpb.VoidGiftCardResponse{
			Result: transactionpb.VoidGiftCardResponse_NOT_FOUND,
		}, nil
	default:
		log.WithError(err).Warn("failure getting gift card account info")
		return nil, status.Error(codes.Internal, "")
	}

	giftCardIssuedIntentRecord, err := s.data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVault.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting gift card issued intent record")
		return nil, status.Error(codes.Internal, "")
	} else if giftCardIssuedIntentRecord.InitiatorOwnerAccount != owner.PublicKey().ToBase58() {
		return &transactionpb.VoidGiftCardResponse{
			Result: transactionpb.VoidGiftCardResponse_DENIED,
		}, nil
	}

	if time.Since(accountInfoRecord.CreatedAt) >= async_account.GiftCardExpiry {
		return &transactionpb.VoidGiftCardResponse{
			Result: transactionpb.VoidGiftCardResponse_OK,
		}, nil
	}

	globalBalanceLock, err := balance.GetOptimisticVersionLock(ctx, s.data, giftCardVault)
	if err != nil {
		log.WithError(err).Warn("failure getting balance lock")
		return nil, status.Error(codes.Internal, "")
	}

	localAccountLock := s.getLocalAccountLock(giftCardVault)
	localAccountLock.Lock()
	defer localAccountLock.Unlock()

	claimedActionRecord, err := s.data.GetGiftCardClaimedAction(ctx, giftCardVault.PublicKey().ToBase58())
	if err == nil {
		mintAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.MintAccount)
		if err != nil {
			log.WithError(err).Warn("invalid mint account")
			return nil, status.Error(codes.Internal, "")
		}

		vmConfig, err := common.GetVmConfigForMint(ctx, s.data, mintAccount)
		if err != nil {
			log.WithError(err).Warn("failure getting vm config")
			return nil, status.Error(codes.Internal, "")
		}

		ownerTimelockAccounts, err := owner.GetTimelockAccounts(vmConfig)
		if err != nil {
			log.WithError(err).Warn("failure getting owner timelock accounts")
			return nil, status.Error(codes.Internal, "")
		}

		if *claimedActionRecord.Destination != ownerTimelockAccounts.Vault.PublicKey().ToBase58() {
			return &transactionpb.VoidGiftCardResponse{
				Result: transactionpb.VoidGiftCardResponse_CLAIMED_BY_OTHER_USER,
			}, nil
		}
		return &transactionpb.VoidGiftCardResponse{
			Result: transactionpb.VoidGiftCardResponse_OK,
		}, nil
	} else if err != action.ErrActionNotFound {
		log.WithError(err).Warn("failure getting gift card claimed action")
		return nil, status.Error(codes.Internal, "")
	}

	err = async_account.InitiateProcessToAutoReturnGiftCard(ctx, s.data, giftCardVault, true, globalBalanceLock)
	if err != nil {
		log.WithError(err).Warn("failure scheduling auto-return action")
		return nil, status.Error(codes.Internal, "")
	}

	// It's ok if this fails, the auto-return worker will just process this account
	// idempotently at a later time
	async_account.MarkAutoReturnCheckComplete(ctx, s.data, accountInfoRecord)

	return &transactionpb.VoidGiftCardResponse{
		Result: transactionpb.VoidGiftCardResponse_OK,
	}, nil
}

func (s *transactionServer) getLocalAccountLock(account *common.Account) *sync.Mutex {
	s.localAccountLocksMu.Lock()
	lock, ok := s.localAccountLocks[account.PublicKey().ToBase58()]
	if !ok {
		lock = &sync.Mutex{}
		s.localAccountLocks[account.PublicKey().ToBase58()] = lock
	}
	s.localAccountLocksMu.Unlock()
	return lock
}
