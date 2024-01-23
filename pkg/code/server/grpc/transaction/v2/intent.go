package transaction_v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/solana/token"
)

const (
	// Assumes the client signature index is consistent across all transactions,
	// including those constructed in the SubmitIntent and Swap RPCs.
	clientSignatureIndex = 1
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

	rendezvousKey, err := common.NewAccountFromPublicKeyString(intentId)
	if err != nil {
		log.WithError(err).Warn("invalid rendezvous key")
		return handleSubmitIntentError(streamer, err)
	}

	marshalled, err := proto.Marshal(submitActionsReq)
	if err == nil {
		log = log.WithField("submit_actions_data_dump", base64.URLEncoding.EncodeToString(marshalled))
	}

	// Figure out what kind of intent we're operating on and initialize the intent handler
	var intentHandler interface{}
	var intentRequiresNewTreasuryPoolFunds bool
	switch submitActionsReq.Metadata.Type.(type) {
	case *transactionpb.Metadata_OpenAccounts:
		log = log.WithField("intent_type", "open_accounts")
		intentHandler = NewOpenAccountsIntentHandler(s.conf, s.data, s.antispamGuard, s.maxmind)
	case *transactionpb.Metadata_SendPrivatePayment:
		log = log.WithField("intent_type", "send_private_payment")
		intentHandler = NewSendPrivatePaymentIntentHandler(s.conf, s.data, s.pusher, s.antispamGuard, s.amlGuard, s.maxmind)
		intentRequiresNewTreasuryPoolFunds = true
	case *transactionpb.Metadata_ReceivePaymentsPrivately:
		log = log.WithField("intent_type", "receive_payments_privately")
		intentHandler = NewReceivePaymentsPrivatelyIntentHandler(s.conf, s.data, s.antispamGuard, s.amlGuard)
		intentRequiresNewTreasuryPoolFunds = true
	case *transactionpb.Metadata_UpgradePrivacy:
		log = log.WithField("intent_type", "upgrade_privacy")
		intentHandler = NewUpgradePrivacyIntentHandler(s.conf, s.data)
	case *transactionpb.Metadata_MigrateToPrivacy_2022:
		log = log.WithField("intent_type", "migrate_to_privacy_2022")
		intentHandler = NewMigrateToPrivacy2022IntentHandler(s.conf, s.data)
	case *transactionpb.Metadata_SendPublicPayment:
		log = log.WithField("intent_type", "send_public_payment")
		intentHandler = NewSendPublicPaymentIntentHandler(s.conf, s.data, s.pusher, s.antispamGuard, s.maxmind)
	case *transactionpb.Metadata_ReceivePaymentsPublicly:
		log = log.WithField("intent_type", "receive_payments_publicly")
		intentHandler = NewReceivePaymentsPubliclyIntentHandler(s.conf, s.data, s.antispamGuard, s.maxmind)
	case *transactionpb.Metadata_EstablishRelationship:
		log = log.WithField("intent_type", "establish_relationship")
		intentHandler = NewEstablishRelationshipIntentHandler(s.conf, s.data, s.antispamGuard)
	default:
		return handleSubmitIntentError(streamer, status.Error(codes.InvalidArgument, "SubmitIntentRequest.SubmitActions.Metadata is nil"))
	}

	var isIntentUpdateOperation bool
	switch intentHandler.(type) {
	case UpdateIntentHandler:
		isIntentUpdateOperation = true
	}

	// The public key that is the owner and signed the intent. This may not be
	// the user depending upon the context of how the user initiated the intent.
	submitActionsOwnerAccount, err := common.NewAccountFromProto(submitActionsReq.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid submit actions owner account")
		return handleSubmitIntentError(streamer, err)
	}
	log = log.WithField("submit_actions_owner_account", submitActionsOwnerAccount.PublicKey().ToBase58())

	// For all allowed cases of owner account types that can call SubmitIntent,
	// we need to find the phone-verified user's 12 words who initiated the intent.
	var initiatorOwnerAccount *common.Account
	var initiatorPhoneNumber *string
	submitActionsOwnerMetadata, err := common.GetOwnerMetadata(ctx, s.data, submitActionsOwnerAccount)
	if err == nil {
		switch submitActionsOwnerMetadata.Type {
		case common.OwnerTypeUser12Words:
			initiatorOwnerAccount = submitActionsOwnerAccount
			initiatorPhoneNumber = &submitActionsOwnerMetadata.VerificationRecord.PhoneNumber
		case common.OwnerTypeRemoteSendGiftCard:
			// Remote send gift cards can only be the owner of an intent for a
			// remote send public receive. In this instance, we need to inspect
			// the destination account, which should be a user's temporary incoming
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
						} else if err == account.ErrAccountInfoNotFound || accountInfoRecord.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
							return newActionValidationError(submitActionsReq.Actions[0], "destination must be a temporary incoming account")
						}

						initiatorOwnerAccount, err = common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
						if err != nil {
							log.WithError(err).Warn("failure getting user initiator owner account")
							return handleSubmitIntentError(streamer, err)
						}

						userOwnerMetadata, err := common.GetOwnerMetadata(ctx, s.data, initiatorOwnerAccount)
						if err != nil {
							log.WithError(err).Warn("failure getting user initiator owner account")
							return handleSubmitIntentError(streamer, err)
						}
						initiatorPhoneNumber = &userOwnerMetadata.VerificationRecord.PhoneNumber
					default:
						return newActionValidationError(submitActionsReq.Actions[0], "expected a no privacy withdraw action")
					}
				}
			}
		default:
			log.Warnf("unhandled owner account type %s", submitActionsOwnerMetadata.Type)
			return handleSubmitIntentError(streamer, errors.New("unhandled owner account type"))
		}
	} else if err == common.ErrOwnerNotFound {
		// Caught by later error
	} else if err != nil {
		log.WithError(err).Warn("failure getting owner account metadata")
		return handleSubmitIntentError(streamer, err)
	}

	// All intents must be initiated by a phone-verified user
	if initiatorOwnerAccount == nil || initiatorPhoneNumber == nil {
		log.Info("intent not initiated by phone-verified user 12 words")
		return handleSubmitIntentError(streamer, ErrNotPhoneVerified)
	}
	log = log.WithField("initiator_owner_account", initiatorOwnerAccount.PublicKey().ToBase58())
	log = log.WithField("initiator_phone_number", *initiatorPhoneNumber)

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
					return handleSubmitIntentError(streamer, newActionValidationErrorf(action, "owner must be %s", authorityAccount.PublicKey().ToBase58()))
				}
			default:
				// Everything else is owned by a user account's 12 words
				if !bytes.Equal(typedAction.OpenAccount.Owner.Value, initiatorOwnerAccount.PublicKey().ToBytes()) {
					return handleSubmitIntentError(streamer, newActionValidationErrorf(action, "owner must be %s", initiatorOwnerAccount.PublicKey().ToBase58()))
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
		InitiatorPhoneNumber:  initiatorPhoneNumber,
		State:                 intent.StateUnknown,
		CreatedAt:             time.Now(),
	}

	// Distributed locking. This is a partial view, since additional locking
	// requirements may not be known until populating intent metadata.
	intentLock := s.intentLocks.Get([]byte(intentId))
	initiatorOwnerLock := s.ownerLocks.Get(initiatorOwnerAccount.PublicKey().ToBytes())
	phoneLock := s.phoneLocks.Get([]byte(*initiatorPhoneNumber))
	intentLock.Lock()
	initiatorOwnerLock.Lock()
	phoneLock.Lock()
	var phoneLockUnlocked bool // Can be unlocked earlier than RPC end
	defer func() {
		if !phoneLockUnlocked {
			phoneLock.Unlock()
		}
		initiatorOwnerLock.Unlock()
		intentLock.Unlock()
	}()

	existingIntentRecord, err := s.data.GetIntent(ctx, intentId)
	if err != intent.ErrIntentNotFound && err != nil {
		log.WithError(err).Warn("failure checking for existing intent record")
		return handleSubmitIntentError(streamer, err)
	}

	if isIntentUpdateOperation {
		// Intent is an update, so ensure we have an existing DB record
		if existingIntentRecord == nil {
			return handleSubmitIntentError(streamer, newIntentValidationError("intent doesn't exists"))
		}

		// Intent is an update, so ensure the original owner account is operating
		// on the intent
		if initiatorOwnerAccount.PublicKey().ToBase58() != existingIntentRecord.InitiatorOwnerAccount {
			return handleSubmitIntentError(streamer, status.Error(codes.PermissionDenied, ""))
		}

		// Validate the update with intent-specific logic
		err = intentHandler.(UpdateIntentHandler).AllowUpdate(ctx, existingIntentRecord, submitActionsReq.Metadata, submitActionsReq.Actions)
		if err != nil {
			switch err.(type) {
			case IntentValidationError:
				log.WithError(err).Warn("intent update failed validation")
			case IntentDeniedError:
				log.WithError(err).Warn("intent update was denied")
			case StaleStateError:
				log.WithError(err).Warn("detected a client with stale state")
			default:
				log.WithError(err).Warn("failure checking if intent update was allowed")
			}
			return handleSubmitIntentError(streamer, err)
		}

		// Use the existing DB record going forward
		intentRecord = existingIntentRecord
	} else {
		// We're operating on a new intent, so validate we don't have an existing DB record
		if existingIntentRecord != nil {
			log.Warn("client is attempting to resubmit an intent or reuse an intent id")
			return handleSubmitIntentError(streamer, newStaleStateError("intent already exists"))
		}

		createIntentHandler := intentHandler.(CreateIntentHandler)

		// Populate metadata into the new DB record
		err = createIntentHandler.PopulateMetadata(ctx, intentRecord, submitActionsReq.Metadata)
		if err != nil {
			log.WithError(err).Warn("failure populating intent metadata")
			return handleSubmitIntentError(streamer, err)
		}

		isNoop, err := createIntentHandler.IsNoop(ctx, intentRecord, submitActionsReq.Metadata, submitActionsReq.Actions)
		if err != nil {
			log.WithError(err).Warn("failure checking if intent is a no-op")
			return handleSubmitIntentError(streamer, err)
		} else if isNoop {
			if err := streamer.Send(okResp); err != nil {
				return handleSubmitIntentError(streamer, err)
			}
			return nil
		}

		// Distributed locking on additional accounts possibly not known until
		// populating intent metadata. Importantly, this must be done prior to
		// doing validation checks in AllowCreation.
		additionalAccountsToLock, err := createIntentHandler.GetAdditionalAccountsToLock(ctx, intentRecord)
		if err != nil {
			return handleSubmitIntentError(streamer, err)
		}

		if additionalAccountsToLock.DestinationOwner != nil {
			destinationOwnerLock := s.ownerLocks.Get(additionalAccountsToLock.DestinationOwner.PublicKey().ToBytes())
			if destinationOwnerLock != initiatorOwnerLock { // Because we're using striped locks
				destinationOwnerLock.Lock()
				defer destinationOwnerLock.Unlock()
			}
		}

		if additionalAccountsToLock.RemoteSendGiftCardVault != nil {
			giftCardLock := s.giftCardLocks.Get(additionalAccountsToLock.RemoteSendGiftCardVault.PublicKey().ToBytes())
			giftCardLock.Lock()
			defer giftCardLock.Unlock()
		}

		var deviceToken *string
		if submitActionsReq.DeviceToken != nil {
			deviceToken = &submitActionsReq.DeviceToken.Value
		}

		// Validate the new intent with intent-specific logic
		err = createIntentHandler.AllowCreation(ctx, intentRecord, submitActionsReq.Metadata, submitActionsReq.Actions, deviceToken)
		if err != nil {
			switch err.(type) {
			case IntentValidationError:
				log.WithError(err).Warn("new intent failed validation")
			case IntentDeniedError:
				log.WithError(err).Warn("new intent was denied")
			case StaleStateError:
				log.WithError(err).Warn("detected a client with stale state")
			default:
				log.WithError(err).Warn("failure checking if new intent was allowed")
			}
			return handleSubmitIntentError(streamer, err)
		}

		// Generically handle treasury pool status to protect ourselves against
		// overly large usage (ie. multiples of the total available funds)
		if intentRequiresNewTreasuryPoolFunds {
			areTreasuryPoolsAvailable, err := s.areAllTreasuryPoolsAvailable(ctx)
			if err != nil {
				return handleSubmitIntentError(streamer, err)
			} else if !areTreasuryPoolsAvailable {
				return status.Error(codes.Unavailable, "temporarily unavailable")
			}
		}
	}

	// Remove the phone lock early, since we only require it for the Allow methods.
	phoneLock.Unlock()
	phoneLockUnlocked = true

	type fulfillmentWithMetadata struct {
		record        *fulfillment.Record
		isRecordSaved bool

		txn               *solana.Transaction
		isCreatedOnDemand bool

		requiresClientSignature bool

		intentOrderingIndexOverriden bool
	}

	// Convert all actions into a set of fulfillments
	var actionHandlers []BaseActionHandler
	var actionRecords []*action.Record
	var fulfillments []fulfillmentWithMetadata
	var reservedNonces []*transaction.SelectedNonce
	var serverParameters []*transactionpb.ServerParameter
	for i, protoAction := range submitActionsReq.Actions {
		log := log.WithField("action_id", i)

		// Figure out what kind of action we're operating on and initialize the
		// action handler
		var actionHandler BaseActionHandler
		var actionType action.Type
		switch typed := protoAction.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			log = log.WithField("action_type", "open_account")
			actionType = action.OpenAccount
			actionHandler, err = NewOpenAccountActionHandler(s.data, typed.OpenAccount, submitActionsReq.Metadata)
		case *transactionpb.Action_CloseEmptyAccount:
			log = log.WithField("action_type", "close_empty_account")
			actionType = action.CloseEmptyAccount
			actionHandler, err = NewCloseEmptyAccountActionHandler(intentRecord.IntentType, typed.CloseEmptyAccount)
		case *transactionpb.Action_CloseDormantAccount:
			log = log.WithField("action_type", "close_dormant_account")
			actionType = action.CloseDormantAccount
			actionHandler, err = NewCloseDormantAccountActionHandler(typed.CloseDormantAccount)
		case *transactionpb.Action_NoPrivacyTransfer:
			log = log.WithField("action_type", "no_privacy_transfer")
			actionType = action.NoPrivacyTransfer
			actionHandler, err = NewNoPrivacyTransferActionHandler(typed.NoPrivacyTransfer)
		case *transactionpb.Action_FeePayment:
			log = log.WithField("action_type", "fee_payment")
			actionType = action.NoPrivacyTransfer
			actionHandler, err = NewFeePaymentActionHandler(typed.FeePayment, s.feeCollector)
		case *transactionpb.Action_NoPrivacyWithdraw:
			log = log.WithField("action_type", "no_privacy_withdraw")
			actionType = action.NoPrivacyWithdraw
			actionHandler, err = NewNoPrivacyWithdrawActionHandler(intentRecord.IntentType, typed.NoPrivacyWithdraw)
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			log = log.WithField("action_type", "temporary_privacy_transfer")
			actionType = action.PrivateTransfer
			actionHandler, err = NewTemporaryPrivacyTransferActionHandler(ctx, s.conf, s.data, intentRecord, protoAction, false, s.selectTreasuryPoolForAdvance)
		case *transactionpb.Action_TemporaryPrivacyExchange:
			log = log.WithField("action_type", "temporary_privacy_exchange")
			actionType = action.PrivateTransfer
			actionHandler, err = NewTemporaryPrivacyTransferActionHandler(ctx, s.conf, s.data, intentRecord, protoAction, true, s.selectTreasuryPoolForAdvance)
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			log = log.WithField("action_type", "permanent_privacy_upgrade")
			actionType = action.PrivateTransfer

			// Pass along the privacy upgrade target found during intent validation
			// to avoid duplication of work.
			cachedUpgradeTarget, ok := intentHandler.(*UpgradePrivacyIntentHandler).GetCachedUpgradeTarget(typed.PermanentPrivacyUpgrade)
			if !ok {
				log.Warn("cached privacy upgrade target not found")
				return handleSubmitIntentError(streamer, errors.New("cached privacy upgrade target not found"))
			}

			actionHandler, err = NewPermanentPrivacyUpgradeActionHandler(
				ctx,
				s.data,
				intentRecord,
				typed.PermanentPrivacyUpgrade,
				cachedUpgradeTarget,
			)
		default:
			return handleSubmitIntentError(streamer, status.Errorf(codes.InvalidArgument, "SubmitIntentRequest.SubmitActions.Actions[%d].Type is nil", i))
		}
		if err != nil {
			log.WithError(err).Warn("failure initializing action handler")
			return handleSubmitIntentError(streamer, errors.New("error initializing action handler"))
		}

		var isUpgradeActionOperation bool
		switch actionHandler.(type) {
		case UpgradeActionHandler:
			isUpgradeActionOperation = true
		}

		// Updates equate to only upgrading existing fulfillments, and vice versa, for now.
		if isUpgradeActionOperation != isIntentUpdateOperation {
			// If we hit this, then we've failed somewhere in the validation code.
			log.Warn("intent update status != action upgrade status")
			return handleSubmitIntentError(streamer, errors.New("intent update status != action upgrade status"))
		}

		actionHandlers = append(actionHandlers, actionHandler)

		// Some actions are optional tools for Code to use at its disposal and
		// are not necessarily required to complete the intent flow. We can
		// choose to discard them by revoking the action immediately. However,
		// clients still have an expectation to sign them, since this is a
		// server toggle. As a result, we must go through the process of creating
		// the transaction.
		areFulfillmentsSavedForAction := true

		// Upgrades cannot create new actions.
		if !isUpgradeActionOperation {
			// Construct the equivalent action record
			actionRecord := &action.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentRecord.IntentType,

				ActionId:   protoAction.Id,
				ActionType: actionType,

				InitiatorPhoneNumber: intentRecord.InitiatorPhoneNumber,

				State: action.StateUnknown,
			}

			err := actionHandler.(CreateActionHandler).PopulateMetadata(actionRecord)
			if err != nil {
				log.WithError(err).Warn("failure populating action metadata")
				return handleSubmitIntentError(streamer, err)
			}

			// Action could be immediately revoked if we opt to not require it
			areFulfillmentsSavedForAction = actionRecord.State != action.StateRevoked

			actionRecords = append(actionRecords, actionRecord)
		}

		// Get action-specific server parameters needed by client to construct the transaction
		serverParameter := actionHandler.GetServerParameter()
		serverParameter.ActionId = protoAction.Id
		serverParameters = append(serverParameters, serverParameter)

		transactionCount := 1
		if !isUpgradeActionOperation {
			transactionCount = actionHandler.(CreateActionHandler).TransactionCount()
		}

		for j := 0; j < transactionCount; j++ {
			var makeTxnResult *makeSolanaTransactionResult
			var selectedNonce *transaction.SelectedNonce
			var actionId uint32
			if isUpgradeActionOperation {
				upgradeActionHandler := actionHandler.(UpgradeActionHandler)

				// Find the fulfillment that is being upgraded.
				fulfillmentToUpgrade := upgradeActionHandler.GetFulfillmentBeingUpgraded()
				if fulfillmentToUpgrade.State != fulfillment.StateUnknown {
					log.Warn("fulfillment being upgraded isn't in the unknown state")
					return handleSubmitIntentError(streamer, errors.New("invalid fulfillment to upgrade"))
				}

				// Re-use the same nonce as the one in the fulfillment we're upgrading,
				// so we avoid server from submitting both.
				selectedNonce, err = transaction.SelectNonceFromFulfillmentToUpgrade(ctx, s.data, fulfillmentToUpgrade)
				if err != nil {
					log.WithError(err).Warn("failure selecting nonce from existing fulfillment")
					return handleSubmitIntentError(streamer, err)
				}
				defer selectedNonce.Unlock()

				// Make a new transaction, which is the upgraded version of the old one.
				makeTxnResult, err = upgradeActionHandler.MakeUpgradedSolanaTransaction(
					selectedNonce.Account,
					selectedNonce.Blockhash,
				)
				if err != nil {
					log.WithError(err).Warn("failure making solana transaction")
					return handleSubmitIntentError(streamer, err)
				}

				actionId = fulfillmentToUpgrade.ActionId
			} else {
				createActionHandler := actionHandler.(CreateActionHandler)

				// Select any available nonce reserved for use for a client transaction,
				// if it's required
				var nonceAccount *common.Account
				var nonceBlockchash solana.Blockhash
				if createActionHandler.RequiresNonce(j) {
					selectedNonce, err = transaction.SelectAvailableNonce(ctx, s.data, nonce.PurposeClientTransaction)
					if err != nil {
						log.WithError(err).Warn("failure selecting available nonce")
						return handleSubmitIntentError(streamer, err)
					}
					defer func() {
						// If we never assign the nonce a signature in the action creation flow,
						// it's safe to put it back in the available pool. The client will have
						// caused a failed RPC call, and we want to avoid malicious or erroneous
						// clients from consuming our nonce pool!
						selectedNonce.ReleaseIfNotReserved()
						selectedNonce.Unlock()
					}()
					nonceAccount = selectedNonce.Account
					nonceBlockchash = selectedNonce.Blockhash
				} else {
					selectedNonce = nil
				}

				// Make a new transaction
				makeTxnResult, err = createActionHandler.MakeNewSolanaTransaction(
					j,
					nonceAccount,
					nonceBlockchash,
				)
				if err != nil {
					log.WithError(err).Warn("failure making solana transaction")
					return handleSubmitIntentError(streamer, err)
				}

				actionId = protoAction.Id
			}

			// Sign the Solana transaction
			if !makeTxnResult.isCreatedOnDemand {
				err = makeTxnResult.txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
				if err != nil {
					log.WithError(err).Warn("failure signing solana transaction")
					return handleSubmitIntentError(streamer, err)
				}
			}

			// Construct the fulfillment record
			fulfillmentRecord := &fulfillment.Record{
				Intent:     intentRecord.IntentId,
				IntentType: intentRecord.IntentType,

				ActionId:   actionId,
				ActionType: actionType,

				FulfillmentType: makeTxnResult.fulfillmentType,

				Source: makeTxnResult.source.PublicKey().ToBase58(),

				IntentOrderingIndex:      0, // Unknown until intent record is saved, so it's injected later
				ActionOrderingIndex:      actionId,
				FulfillmentOrderingIndex: makeTxnResult.fulfillmentOrderingIndex,

				DisableActiveScheduling: makeTxnResult.disableActiveScheduling,

				InitiatorPhoneNumber: intentRecord.InitiatorPhoneNumber,

				State: fulfillment.StateUnknown,
			}
			if !makeTxnResult.isCreatedOnDemand {
				fulfillmentRecord.Data = makeTxnResult.txn.Marshal()
				fulfillmentRecord.Signature = pointer.String(base58.Encode(makeTxnResult.txn.Signature()))

				fulfillmentRecord.Nonce = pointer.String(selectedNonce.Account.PublicKey().ToBase58())
				fulfillmentRecord.Blockhash = pointer.String(base58.Encode(selectedNonce.Blockhash[:]))
			}
			if makeTxnResult.destination != nil {
				destination := makeTxnResult.destination.PublicKey().ToBase58()
				fulfillmentRecord.Destination = &destination
			}
			if makeTxnResult.intentOrderingIndexOverride != nil {
				fulfillmentRecord.IntentOrderingIndex = *makeTxnResult.intentOrderingIndexOverride
			}
			if makeTxnResult.actionOrderingIndexOverride != nil {
				fulfillmentRecord.ActionOrderingIndex = *makeTxnResult.actionOrderingIndexOverride
			}

			// Transaction requires a client signature
			var requiresClientSignature bool
			if !makeTxnResult.isCreatedOnDemand && makeTxnResult.txn.Message.Header.NumSignatures > clientSignatureIndex {
				// Upgraded transactions always use the same nonce, so there's no
				// need to provide it.
				if !isUpgradeActionOperation {
					serverParameter.Nonces = append(serverParameter.Nonces, &transactionpb.NoncedTransactionMetadata{
						Nonce: selectedNonce.Account.ToProto(),
						Blockhash: &commonpb.Blockhash{
							Value: selectedNonce.Blockhash[:],
						},
					})
				}

				requiresClientSignature = true
			}

			fulfillments = append(fulfillments, fulfillmentWithMetadata{
				record: fulfillmentRecord,

				isCreatedOnDemand: makeTxnResult.isCreatedOnDemand,
				txn:               makeTxnResult.txn,

				requiresClientSignature: requiresClientSignature,

				intentOrderingIndexOverriden: makeTxnResult.intentOrderingIndexOverride != nil,

				isRecordSaved: areFulfillmentsSavedForAction,
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

	var unsignedFulfillments []fulfillmentWithMetadata
	for _, fulfillmentWithMetadata := range fulfillments {
		if fulfillmentWithMetadata.requiresClientSignature {
			unsignedFulfillments = append(unsignedFulfillments, fulfillmentWithMetadata)
		}
	}

	metricsIntentTypeValue := intentRecord.IntentType.String()
	if isIntentUpdateOperation {
		metricsIntentTypeValue = "upgrade_privacy"
	}
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
				unsignedFulfillment.txn.Message.Accounts[clientSignatureIndex],
				unsignedFulfillment.txn.Message.Marshal(),
				signature.Value,
			) {
				signatureErrorDetails = append(signatureErrorDetails, toInvalidSignatureErrorDetails(unsignedFulfillment.record.ActionId, *unsignedFulfillment.txn, signature))
			}

			copy(unsignedFulfillment.txn.Signatures[clientSignatureIndex][:], signature.Value)
			unsignedFulfillments[i].record.Data = unsignedFulfillments[i].txn.Marshal()
		}

		if len(signatureErrorDetails) > 0 {
			return handleSubmitIntentStructuredError(
				streamer,
				transactionpb.SubmitIntentResponse_Error_SIGNATURE_ERROR,
				signatureErrorDetails...,
			)
		}
	}

	var chatMessagesToPush []*chat_util.MessageWithOwner

	// Save all of the required DB records in one transaction to complete the
	// intent operation. It's very bad if we end up failing halfway through.
	//
	// Note: This is the first use case of this new method to do this kind of
	// operation. Not all store implementations have real support for this, so
	// if anything is added, then ensure it does!
	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		// Updates cannot create new intent or action records, yet.
		if !isIntentUpdateOperation {
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
		}

		// Save all fulfillment records
		fulfillmentRecordsToSave := make([]*fulfillment.Record, 0)
		for i, fulfillmentWithMetadata := range fulfillments {
			if !fulfillmentWithMetadata.isRecordSaved {
				continue
			}

			// If the intent ordering index isn't overriden, the inject it here where
			// the value is guaranteed to be set due to the lazy saving of the intent
			// record.
			if !fulfillmentWithMetadata.intentOrderingIndexOverriden {
				fulfillmentWithMetadata.record.IntentOrderingIndex = intentRecord.Id
			}

			fulfillmentRecordsToSave = append(fulfillmentRecordsToSave, fulfillmentWithMetadata.record)

			// Reserve the nonce with the latest server-signed fulfillment.
			if !fulfillmentWithMetadata.isCreatedOnDemand {
				nonceToReserve := reservedNonces[i]
				if isIntentUpdateOperation {
					err = nonceToReserve.UpdateSignature(ctx, *fulfillmentWithMetadata.record.Signature)
				} else {
					err = nonceToReserve.MarkReservedWithSignature(ctx, *fulfillmentWithMetadata.record.Signature)
				}
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

		// Save additional state related to each action
		for _, actionHandler := range actionHandlers {
			err = actionHandler.OnSaveToDB(ctx)
			if err != nil {
				log.WithError(err).Warn("failure executing action db save callback handler")
				return err
			}
		}

		// Updates apply to intents that have already been created, and consequently,
		// been previously moved to the pending state.
		if !isIntentUpdateOperation {
			// Save additional state related to the intent
			err = intentHandler.(CreateIntentHandler).OnSaveToDB(ctx, intentRecord)
			if err != nil {
				log.WithError(err).Warn("failure executing intent db save callback")
				return err
			}

			// Update various chats with exchange data messages
			err = chat_util.SendCashTransactionsExchangeMessage(ctx, s.data, intentRecord)
			if err != nil {
				log.WithError(err).Warn("failure updating cash transaction chat")
				return err
			}
			chatMessagesToPush, err = chat_util.SendMerchantExchangeMessage(ctx, s.data, intentRecord, actionRecords)
			if err != nil {
				log.WithError(err).Warn("failure updating merchant chat")
				return err
			}

			// Mark the intent as pending once everything else has succeeded
			err = s.markIntentAsPending(ctx, intentRecord)
			if err != nil {
				log.WithError(err).Warn("failure marking the intent as pending")
				return err
			}

			// Mark the associated webhook as pending, if it was registered
			err = s.markWebhookAsPending(ctx, intentRecord.IntentId)
			if err != nil {
				log.WithError(err).Warn("failure marking webhook as pending")
				return err
			}

			// Create a message on the intent ID to indicate the intent was submitted
			//
			// Note: This function only errors on the DB save, and not forwarding, which
			//       is ideal for this use case.
			//
			// todo: We could also make this an account update event by creating the message
			//       on each involved owner accounts' stream.
			_, err = s.messagingClient.InternallyCreateMessage(ctx, rendezvousKey, &messagingpb.Message{
				Kind: &messagingpb.Message_IntentSubmitted{
					IntentSubmitted: &messagingpb.IntentSubmitted{
						IntentId: submitActionsReq.Id,
						Metadata: submitActionsReq.Metadata,
					},
				},
			})
			if err != nil {
				log.WithError(err).Warn("failure creating intent submitted message")
				return err
			}
		}

		return nil
	})
	if err != nil {
		return handleSubmitIntentError(streamer, err)
	}

	//
	// Intent is submitted, and anything beyond this point is best-effort.
	// We must send success back to the client. Rolling back the intent is
	// not an option here, since it's already being processed by workers.
	//

	log.Debug("intent submitted")

	// Post-processing when an intent has been committed to the DB.
	if !isIntentUpdateOperation {
		err = intentHandler.(CreateIntentHandler).OnCommittedToDB(ctx, intentRecord)
		if err != nil {
			log.WithError(err).Warn("failure executing intent committed callback handler handler")
		}

		if len(chatMessagesToPush) > 0 {
			go func() {
				for _, chatMessageToPush := range chatMessagesToPush {
					push.SendChatMessagePushNotification(
						context.TODO(),
						s.data,
						s.pusher,
						chatMessageToPush.Title,
						chatMessageToPush.Owner,
						chatMessageToPush.Message,
					)
				}
			}()
		}
	}

	// Fire off some success metrics
	if !isIntentUpdateOperation {
		recordUserIntentCreatedEvent(ctx, intentRecord)
	}
	switch submitActionsReq.Metadata.Type.(type) {
	case *transactionpb.Metadata_UpgradePrivacy:
		recordPrivacyUpgradedEvent(ctx, intentRecord, len(submitActionsReq.Actions))
	}

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

	// There are no intent-based airdrops ATM
	if false {
		backgroundCtx := context.Background()

		// todo: generic metrics utility for this
		nr, ok := ctx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
		if ok {
			backgroundCtx = context.WithValue(backgroundCtx, metrics.NewRelicContextKey, nr)
		}

		// todo: We likely want to put this in a worker if this is a long term feature
		if s.conf.enableAirdrops.Get(backgroundCtx) {
			if s.conf.enableAsyncAirdropProcessing.Get(backgroundCtx) {
				go s.maybeAirdropForSubmittingIntent(backgroundCtx, intentRecord, submitActionsOwnerMetadata)
			} else {
				s.maybeAirdropForSubmittingIntent(backgroundCtx, intentRecord, submitActionsOwnerMetadata)
			}
		}
	}

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

func (s *transactionServer) markIntentAsPending(ctx context.Context, record *intent.Record) error {
	if record.State != intent.StateUnknown {
		return nil
	}

	// After one minute, we mark the intent as revoked, so avoid the race with
	// a time-based check until we have distributed locks
	if time.Since(record.CreatedAt) > time.Minute {
		return errors.New("took too long to mark intent as pending")
	}

	record.State = intent.StatePending
	return s.data.SaveIntent(ctx, record)
}

func (s *transactionServer) markWebhookAsPending(ctx context.Context, id string) error {
	webhookRecord, err := s.data.GetWebhook(ctx, id)
	if err == webhook.ErrNotFound {
		return nil
	} else if err != nil {
		return err
	}

	if webhookRecord.State != webhook.StateUnknown {
		return nil
	}

	webhookRecord.NextAttemptAt = pointer.Time(time.Now())
	webhookRecord.State = webhook.StatePending
	return s.data.UpdateWebhook(ctx, webhookRecord)
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
	case intent.SendPrivatePayment:
		destinationOwnerAccount = intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount
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
	case intent.OpenAccounts:
		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_OpenAccounts{
				OpenAccounts: &transactionpb.OpenAccountsMetadata{},
			},
		}
	case intent.SendPrivatePayment:
		destinationAccount, err := common.NewAccountFromPublicKeyString(intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
		if err != nil {
			log.WithError(err).Warn("invalid destination account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_SendPrivatePayment{
				SendPrivatePayment: &transactionpb.SendPrivatePaymentMetadata{
					Destination: destinationAccount.ToProto(),
					ExchangeData: &transactionpb.ExchangeData{
						Currency:     strings.ToLower(string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency)),
						ExchangeRate: intentRecord.SendPrivatePaymentMetadata.ExchangeRate,
						NativeAmount: intentRecord.SendPrivatePaymentMetadata.ExchangeRate * float64(kin.FromQuarks(intentRecord.SendPrivatePaymentMetadata.Quantity)),
						Quarks:       intentRecord.SendPrivatePaymentMetadata.Quantity,
					},
					IsWithdrawal: intentRecord.SendPrivatePaymentMetadata.IsWithdrawal,
					IsRemoteSend: intentRecord.SendPrivatePaymentMetadata.IsRemoteSend,
				},
			},
		}
	case intent.ReceivePaymentsPrivately:
		sourceAccount, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPrivatelyMetadata.Source)
		if err != nil {
			log.WithError(err).Warn("invalid source account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_ReceivePaymentsPrivately{
				ReceivePaymentsPrivately: &transactionpb.ReceivePaymentsPrivatelyMetadata{
					Source:    sourceAccount.ToProto(),
					Quarks:    intentRecord.ReceivePaymentsPrivatelyMetadata.Quantity,
					IsDeposit: intentRecord.ReceivePaymentsPrivatelyMetadata.IsDeposit,
				},
			},
		}
	case intent.MigrateToPrivacy2022:
		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_MigrateToPrivacy_2022{
				MigrateToPrivacy_2022: &transactionpb.MigrateToPrivacy2022Metadata{
					Quarks: intentRecord.MigrateToPrivacy2022Metadata.Quantity,
				},
			},
		}
	case intent.SendPublicPayment:
		destinationAccount, err := common.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
		if err != nil {
			log.WithError(err).Warn("invalid destination account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_SendPublicPayment{
				SendPublicPayment: &transactionpb.SendPublicPaymentMetadata{
					Destination: destinationAccount.ToProto(),
					ExchangeData: &transactionpb.ExchangeData{
						Currency:     strings.ToLower(string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency)),
						ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
						NativeAmount: intentRecord.SendPublicPaymentMetadata.ExchangeRate * float64(kin.FromQuarks(intentRecord.SendPublicPaymentMetadata.Quantity)),
						Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
					},
					IsWithdrawal: intentRecord.SendPublicPaymentMetadata.IsWithdrawal,
				},
			},
		}
	case intent.ReceivePaymentsPublicly:
		sourceAccount, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
		if err != nil {
			log.WithError(err).Warn("invalid destination account")
			return nil, status.Error(codes.Internal, "")
		}

		metadata = &transactionpb.Metadata{
			Type: &transactionpb.Metadata_ReceivePaymentsPublicly{
				ReceivePaymentsPublicly: &transactionpb.ReceivePaymentsPubliclyMetadata{
					Source:                  sourceAccount.ToProto(),
					Quarks:                  intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
					IsRemoteSend:            intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend,
					IsIssuerVoidingGiftCard: intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard,
					ExchangeData: &transactionpb.ExchangeData{
						Currency:     string(intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency),
						ExchangeRate: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate,
						NativeAmount: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount,
						Quarks:       intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
					},
				},
			},
		}
	default:
		// This is not a client-initiated intent type. Don't reveal anything.
		return &transactionpb.GetIntentMetadataResponse{
			Result: transactionpb.GetIntentMetadataResponse_NOT_FOUND,
		}, nil
	}

	return &transactionpb.GetIntentMetadataResponse{
		Result:   transactionpb.GetIntentMetadataResponse_OK,
		Metadata: metadata,
	}, nil
}

// todo: Test the blockchain checks when we have a mocked Solana client
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

	//
	// Part 1: Is this a legacy Code timelock account? If so, deny it.
	//

	timelockRecord, err := s.data.GetTimelockByVault(ctx, accountToCheck.PublicKey().ToBase58())
	switch err {
	case nil:
		if timelockRecord.DataVersion != timelock_token_v1.DataVersion1 {
			return &transactionpb.CanWithdrawToAccountResponse{
				IsValidPaymentDestination: false,
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			}, nil
		}
	case timelock.ErrTimelockNotFound:
		// Nothing to do
	default:
		log.WithError(err).Warn("failure checking timelock db")
		return nil, status.Error(codes.Internal, "")
	}

	//
	// Part 2: Is this a privacy-based timelock vault? If so, only allow primary accounts.
	//

	if timelockRecord != nil {
		accountInfoRecord, err := s.data.GetAccountInfoByTokenAddress(ctx, accountToCheck.PublicKey().ToBase58())
		if err == nil {
			// todo: may need to check if we're going to close the primary account when supported in the future
			return &transactionpb.CanWithdrawToAccountResponse{
				AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
				IsValidPaymentDestination: accountInfoRecord.AccountType == commonpb.AccountType_PRIMARY || accountInfoRecord.AccountType == commonpb.AccountType_RELATIONSHIP,
			}, nil
		} else {
			log.WithError(err).Warn("failure checking account info db")
			return nil, status.Error(codes.Internal, "")
		}
	}

	//
	// Part 3: Is this an opened Kin token account? If so, allow it.
	//

	_, err = s.data.GetBlockchainTokenAccountInfo(ctx, accountToCheck.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		return &transactionpb.CanWithdrawToAccountResponse{
			AccountType:               transactionpb.CanWithdrawToAccountResponse_TokenAccount,
			IsValidPaymentDestination: true,
		}, nil
	case token.ErrAccountNotFound, solana.ErrNoAccountInfo, token.ErrInvalidTokenAccount:
		// Nothing to do
	default:
		log.WithError(err).Warn("failure checking against blockchain as a token account")
		return nil, status.Error(codes.Internal, "")
	}

	//
	// Part 4: Is this an owner account with an opened Kin ATA? If so, allow it.
	//

	ata, err := accountToCheck.ToAssociatedTokenAccount(common.KinMintAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting ata address")
		return nil, status.Error(codes.Internal, "")
	}

	var requiresInitialization bool
	_, err = s.data.GetBlockchainTokenAccountInfo(ctx, ata.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		return &transactionpb.CanWithdrawToAccountResponse{
			IsValidPaymentDestination: true,
			AccountType:               transactionpb.CanWithdrawToAccountResponse_OwnerAccount,
		}, nil
	case token.ErrAccountNotFound, solana.ErrNoAccountInfo:
		// ATA doesn't exist, and we won't be subsidizing it. Let the client know
		// they should initialize it first.
		requiresInitialization = true
	case token.ErrInvalidTokenAccount:
		// Nothing to do
	default:
		log.WithError(err).Warn("failure checking against blockchain as an owner account")
		return nil, status.Error(codes.Internal, "")
	}

	return &transactionpb.CanWithdrawToAccountResponse{
		AccountType:               transactionpb.CanWithdrawToAccountResponse_Unknown,
		IsValidPaymentDestination: false,
		RequiresInitialization:    requiresInitialization,
	}, nil
}

func (s *transactionServer) GetPrivacyUpgradeStatus(ctx context.Context, req *transactionpb.GetPrivacyUpgradeStatusRequest) (*transactionpb.GetPrivacyUpgradeStatusResponse, error) {
	intentId := base58.Encode(req.IntentId.Value)

	log := s.log.WithFields(logrus.Fields{
		"method": "GetPrivacyUpgradeStatus",
		"intent": intentId,
		"action": req.ActionId,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	var result transactionpb.GetPrivacyUpgradeStatusResponse_Result
	var upgradeStatus transactionpb.GetPrivacyUpgradeStatusResponse_Status

	// The status is directly tied to our ability to select another commitment
	// account to redirect the repayment to
	_, err := selectCandidateForPrivacyUpgrade(ctx, s.data, intentId, req.ActionId)
	switch err {
	case intent.ErrIntentNotFound:
		result = transactionpb.GetPrivacyUpgradeStatusResponse_INTENT_NOT_FOUND
	case action.ErrActionNotFound:
		result = transactionpb.GetPrivacyUpgradeStatusResponse_ACTION_NOT_FOUND
	case ErrInvalidActionToUpgrade:
		result = transactionpb.GetPrivacyUpgradeStatusResponse_INVALID_ACTION
	case ErrPrivacyUpgradeMissed:
		upgradeStatus = transactionpb.GetPrivacyUpgradeStatusResponse_TEMPORARY_TRANSACTION_FINALIZED
	case ErrPrivacyAlreadyUpgraded:
		upgradeStatus = transactionpb.GetPrivacyUpgradeStatusResponse_ALREADY_UPGRADED
	case ErrWaitForNextBlock:
		upgradeStatus = transactionpb.GetPrivacyUpgradeStatusResponse_WAITING_FOR_NEXT_BLOCK
	case nil:
		upgradeStatus = transactionpb.GetPrivacyUpgradeStatusResponse_READY_FOR_UPGRADE
	default:
		log.WithError(err).Warn("failure trying to select candidate for privacy upgrade")
		return nil, status.Error(codes.Internal, "")
	}

	return &transactionpb.GetPrivacyUpgradeStatusResponse{
		Result: result,
		Status: upgradeStatus,
	}, nil
}

// todo: This doesn't prioritize anything right now, and that's probably ok anyways.
func (s *transactionServer) GetPrioritizedIntentsForPrivacyUpgrade(ctx context.Context, req *transactionpb.GetPrioritizedIntentsForPrivacyUpgradeRequest) (*transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse, error) {
	log := s.log.WithField("method", "GetPrioritizedIntentsForPrivacyUpgrade")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	// Get a set of commitment records that look like they can be upgraded.
	// We simply pick the first limit number of commitments by an owner.
	// Earlier commitments have a better chance of having an available merkle
	// proof.
	//
	// Get a small number of payments worth of commitments. A payment in the
	// worst case has ~15 actions for a send or receive. Since a client has
	// limited time to upgrade, it makes sense to not delay by processing too
	// many commitments.
	commitmentRecords, err := s.data.GetUpgradeableCommitmentsByOwner(ctx, owner.PublicKey().ToBase58(), 15)
	if err == commitment.ErrCommitmentNotFound {
		return &transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse{
			Result: transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting commitment records")
		return nil, status.Error(codes.Internal, "")
	}

	// Filter out commitment records that don't have a merkle proof. These
	// can't be upgraded right now.
	var commitmentRecordsWithProofs []*commitment.Record
	for _, commitmentRecord := range commitmentRecords {
		log := log.WithField("commitment", commitmentRecord.Address)

		canUpgrade, err := canUpgradeCommitmentAction(ctx, s.data, commitmentRecord)
		if err != nil {
			log.WithError(err).Warn("failure checking if commitment action can be upgraded")
			return nil, status.Error(codes.Internal, "")
		} else if canUpgrade {
			commitmentRecordsWithProofs = append(commitmentRecordsWithProofs, commitmentRecord)
		}
	}

	// There's nothing to upgrade right now
	if len(commitmentRecordsWithProofs) == 0 {
		return &transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse{
			Result: transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_NOT_FOUND,
		}, nil
	}

	limit := int(req.Limit)
	if limit == 0 {
		limit = 10
	}

	commitmentsByIntent := make(map[string][]*commitment.Record)
	for _, commitmentRecord := range commitmentRecordsWithProofs {
		// Keep within limits
		_, exists := commitmentsByIntent[commitmentRecord.Intent]
		if !exists && len(commitmentsByIntent) >= limit {
			continue
		}

		commitmentsByIntent[commitmentRecord.Intent] = append(commitmentsByIntent[commitmentRecord.Intent], commitmentRecord)
	}

	// Convert to proto models
	var items []*transactionpb.UpgradeableIntent
	for intentId, commitmentRecords := range commitmentsByIntent {
		log := log.WithField("intent", intentId)

		item, err := toUpgradeableIntentProto(ctx, s.data, intentId, commitmentRecords)
		if err != nil {
			log.WithError(err).Warn("failure converting commitment records to an upgradeable intent proto message")
			return nil, status.Error(codes.Internal, "")
		}

		items = append(items, item)
	}

	return &transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse{
		Result: transactionpb.GetPrioritizedIntentsForPrivacyUpgradeResponse_OK,
		Items:  items,
	}, nil
}

func toUpgradeableIntentProto(ctx context.Context, data code_data.Provider, intentId string, commitmentRecords []*commitment.Record) (*transactionpb.UpgradeableIntent, error) {
	intentIDBytes, err := base58.Decode(intentId)
	if err != nil {
		return nil, err
	}

	var actions []*transactionpb.UpgradeableIntent_UpgradeablePrivateAction
	for _, commitmentRecord := range commitmentRecords {
		if commitmentRecord.Intent != intentId {
			return nil, errors.New("commitment intent id doesn't match")
		}

		fulfillmentRecords, err := data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, commitmentRecord.Intent, commitmentRecord.ActionId)
		if err != nil {
			return nil, err
		}

		if len(fulfillmentRecords) != 1 || *fulfillmentRecords[0].Destination != commitmentRecord.Vault {
			return nil, errors.New("fulfillment to upgrade was not found")
		}
		fulfillmentToUpgrade := fulfillmentRecords[0]

		var txn solana.Transaction
		err = txn.Unmarshal(fulfillmentToUpgrade.Data)
		if err != nil {
			return nil, err
		}

		clientSignature := txn.Signatures[clientSignatureIndex]

		// Clear out all signatures, so clients have no way of submitting this transaction
		var emptySig solana.Signature
		for i := range txn.Signatures {
			copy(txn.Signatures[i][:], emptySig[:])
		}

		// todo: this can be heavily cached
		sourceAccountInfo, err := data.GetAccountInfoByTokenAddress(ctx, fulfillmentToUpgrade.Source)
		if err != nil {
			return nil, err
		}

		originalDestination, err := common.NewAccountFromPublicKeyString(commitmentRecord.Destination)
		if err != nil {
			return nil, err
		}

		treasuryPool, err := common.NewAccountFromPublicKeyString(commitmentRecord.Pool)
		if err != nil {
			return nil, err
		}

		recentRootBytes, err := hex.DecodeString(commitmentRecord.RecentRoot)
		if err != nil {
			return nil, err
		}

		action := &transactionpb.UpgradeableIntent_UpgradeablePrivateAction{
			TransactionBlob: &commonpb.Transaction{
				Value: txn.Marshal(),
			},
			ClientSignature: &commonpb.Signature{
				Value: clientSignature[:],
			},
			ActionId:              commitmentRecord.ActionId,
			SourceAccountType:     sourceAccountInfo.AccountType,
			SourceDerivationIndex: sourceAccountInfo.Index,
			OriginalDestination:   originalDestination.ToProto(),
			OriginalAmount:        commitmentRecord.Amount,
			Treasury:              treasuryPool.ToProto(),
			RecentRoot: &commonpb.Hash{
				Value: recentRootBytes,
			},
		}
		actions = append(actions, action)
	}

	return &transactionpb.UpgradeableIntent{
		Id: &commonpb.IntentId{
			Value: intentIDBytes,
		},
		Actions: actions,
	}, nil
}
