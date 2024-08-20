package transaction_v2

import (
	"bytes"
	"context"
	"math"
	"strings"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/oschwald/maxminddb-golang"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/event"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	event_util "github.com/code-payments/code-server/pkg/code/event"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/code/lawenforcement"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

// todo: Make working with different timelock versions easier

var accountTypesToOpen = []commonpb.AccountType{
	commonpb.AccountType_PRIMARY,
	commonpb.AccountType_TEMPORARY_INCOMING,
	commonpb.AccountType_TEMPORARY_OUTGOING,
	commonpb.AccountType_BUCKET_1_KIN,
	commonpb.AccountType_BUCKET_10_KIN,
	commonpb.AccountType_BUCKET_100_KIN,
	commonpb.AccountType_BUCKET_1_000_KIN,
	commonpb.AccountType_BUCKET_10_000_KIN,
	commonpb.AccountType_BUCKET_100_000_KIN,
	commonpb.AccountType_BUCKET_1_000_000_KIN,
}

var bucketSizeByAccountType = map[commonpb.AccountType]uint64{
	commonpb.AccountType_BUCKET_1_KIN:         kin.ToQuarks(1),
	commonpb.AccountType_BUCKET_10_KIN:        kin.ToQuarks(10),
	commonpb.AccountType_BUCKET_100_KIN:       kin.ToQuarks(100),
	commonpb.AccountType_BUCKET_1_000_KIN:     kin.ToQuarks(1_000),
	commonpb.AccountType_BUCKET_10_000_KIN:    kin.ToQuarks(10_000),
	commonpb.AccountType_BUCKET_100_000_KIN:   kin.ToQuarks(100_000),
	commonpb.AccountType_BUCKET_1_000_000_KIN: kin.ToQuarks(1_000_000),
}

var allowedBucketQuarkAmounts map[uint64]struct{}

func init() {
	allowedBucketQuarkAmounts = make(map[uint64]struct{})
	for _, bucketSize := range bucketSizeByAccountType {
		for multiple := uint64(1); multiple < 10; multiple++ {
			allowedBucketQuarkAmounts[multiple*bucketSize] = struct{}{}
		}
	}
}

type lockableAccounts struct {
	DestinationOwner        *common.Account
	RemoteSendGiftCardVault *common.Account
}

// CreateIntentHandler is an interface for handling new intent creations
type CreateIntentHandler interface {
	// PopulateMetadata adds intent metadata to the provided intent record
	// using the client-provided protobuf variant. No other fields in the
	// intent should be modified.
	PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error

	// IsNoop determines whether the intent is a no-op operation. SubmitIntent will
	// simply return OK and stop any further intent processing.
	//
	// Note: This occurs before validation, so if anything appears out-of-order, then
	// the recommendation is to return false and have the verification logic catch the
	// error.
	IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error)

	// GetAdditionalAccountsToLock gets additional accounts to apply distributed
	// locking on that are specific to an intent.
	//
	// Note: Assumes relevant information is contained in the intent record after
	// calling PopulateMetadata.
	GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error)

	// AllowCreation determines whether the new intent creation should be allowed.
	AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error

	// OnSaveToDB is a callback when the intent is being saved to the DB
	// within the scope of a DB transaction. Additional supporting DB records
	// (ie. not the intent record) relevant to the intent should be saved here.
	OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error

	// OnCommittedToDB is a callback when the intent has been committed to the
	// DB. Any instant side-effects should called here, and can be done async
	// in a new goroutine to not affect SubmitIntent latency.
	//
	// Note: Any errors generated here have no effect on rolling back the intent.
	//       This is all best-effort up to this point. Use a worker for things
	//       requiring retries!
	OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error
}

// UpdateIntentHandler is an interface for handling updates to an existing intent
type UpdateIntentHandler interface {
	// AllowUpdate determines whether an intent update should be allowed.
	AllowUpdate(ctx context.Context, existingIntent *intent.Record, metdata *transactionpb.Metadata, actions []*transactionpb.Action) error
}

type OpenAccountsIntentHandler struct {
	conf                    *conf
	data                    code_data.Provider
	antispamGuard           *antispam.Guard
	antispamSuccessCallback func() error
	maxmind                 *maxminddb.Reader
}

func NewOpenAccountsIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard, maxmind *maxminddb.Reader) CreateIntentHandler {
	return &OpenAccountsIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		maxmind:       maxmind,
	}
}

func (h *OpenAccountsIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetOpenAccounts()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	intentRecord.IntentType = intent.OpenAccounts
	intentRecord.OpenAccountsMetadata = &intent.OpenAccountsMetadata{}

	return nil
}

func (h *OpenAccountsIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return false, err
	}

	_, err = h.data.GetLatestIntentByInitiatorAndType(ctx, intent.OpenAccounts, initiatiorOwnerAccount.PublicKey().ToBase58())
	if err == nil {
		return true, nil
	} else if err != intent.ErrIntentNotFound {
		return false, err
	}

	return false, nil
}

func (h *OpenAccountsIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	return &lockableAccounts{}, nil
}

func (h *OpenAccountsIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := metadata.GetOpenAccounts()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	//
	// Part 1: Intent ID validation
	//

	err = validateIntentIdIsNotRequest(ctx, h.data, intentRecord.IntentId)
	if err != nil {
		return err
	}

	//
	// Part 2: Antispam checks against the phone number
	//

	if !h.conf.disableAntispamChecks.Get(ctx) {
		allow, reason, successCallback, err := h.antispamGuard.AllowOpenAccounts(ctx, initiatiorOwnerAccount, deviceToken)
		if err != nil {
			return err
		} else if !allow {
			return newIntentDeniedErrorWithAntispamReason(reason, "antispam guard denied account creation")
		}
		h.antispamSuccessCallback = successCallback
	}

	//
	// Part 3: Validate the owner hasn't already created an OpenAccounts intent
	//

	_, err = h.data.GetLatestIntentByInitiatorAndType(ctx, intent.OpenAccounts, initiatiorOwnerAccount.PublicKey().ToBase58())
	if err == nil {
		return newStaleStateError("already submitted intent to open accounts")
	} else if err != intent.ErrIntentNotFound {
		return err
	}

	//
	// Part 4: Validate the individual actions
	//

	err = h.validateActions(initiatiorOwnerAccount, actions)
	if err != nil {
		return err
	}

	//
	// Part 5: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 6: Validate fee payments
	//

	return validateFeePayments(ctx, h.data, intentRecord, simResult)
}

func (h *OpenAccountsIntentHandler) validateActions(initiatiorOwnerAccount *common.Account, actions []*transactionpb.Action) error {
	expectedActionCount := len(accountTypesToOpen)
	if len(actions) != expectedActionCount {
		return newIntentValidationErrorf("expected %d total actions", expectedActionCount)
	}

	for i, expectedAccountType := range accountTypesToOpen {
		openAction := actions[i]

		if openAction.GetOpenAccount() == nil {
			return newActionValidationError(openAction, "expected an open account action")
		}

		if openAction.GetOpenAccount().AccountType != expectedAccountType {
			return newActionValidationErrorf(openAction, "account type must be %s", expectedAccountType)
		}

		if openAction.GetOpenAccount().Index != 0 {
			return newActionValidationError(openAction, "index must be 0 for all newly opened accounts")
		}

		if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatiorOwnerAccount.PublicKey().ToBytes()) {
			return newActionValidationErrorf(openAction, "owner must be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
		}

		switch expectedAccountType {
		case commonpb.AccountType_PRIMARY:
			if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
				return newActionValidationErrorf(openAction, "authority must be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
			}
		default:
			if bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
				return newActionValidationErrorf(openAction, "authority cannot be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
			}
		}

		expectedVaultAccount, err := getExpectedTimelockVaultFromProtoAccount(openAction.GetOpenAccount().Authority)
		if err != nil {
			return err
		}

		if !bytes.Equal(openAction.GetOpenAccount().Token.Value, expectedVaultAccount.PublicKey().ToBytes()) {
			return newActionValidationErrorf(openAction, "token must be %s", expectedVaultAccount.PublicKey().ToBase58())
		}
	}

	return nil
}

func (h *OpenAccountsIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	userAgent, err := client.GetUserAgent(ctx)
	if err != nil {
		// Should fail much earlier in the account creation flow than here
		return err
	}

	// Only iOS is eligible for airdrops until we can get stable device IDs
	// for Android.
	//
	// Note: Device attestation guarantees the user agent matches the device
	//       type that generated the token.
	if userAgent.DeviceType != client.DeviceTypeIOS {
		err := h.data.MarkIneligibleForAirdrop(ctx, intentRecord.InitiatorOwnerAccount)
		if err != nil {
			return err
		}
	}

	eventRecord := &event.Record{
		EventId:   intentRecord.IntentId,
		EventType: event.AccountCreated,

		SourceCodeAccount: intentRecord.InitiatorOwnerAccount,

		SourceIdentity: *intentRecord.InitiatorPhoneNumber,

		SpamConfidence: 0,

		CreatedAt: time.Now(),
	}
	event_util.InjectClientDetails(ctx, h.maxmind, eventRecord, true)

	return h.data.SaveEvent(ctx, eventRecord)
}

func (h *OpenAccountsIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	if h.antispamSuccessCallback != nil {
		// todo: Something more robust, since this is part of the fire & forget
		//       portion of SubmitIntent
		return h.antispamSuccessCallback()
	}

	return nil
}

type SendPrivatePaymentIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	pusher        push_lib.Provider
	antispamGuard *antispam.Guard
	amlGuard      *lawenforcement.AntiMoneyLaunderingGuard
	maxmind       *maxminddb.Reader

	cachedPaymentRequestRequest *paymentrequest.Record
}

func NewSendPrivatePaymentIntentHandler(
	conf *conf,
	data code_data.Provider,
	pusher push_lib.Provider,
	antispamGuard *antispam.Guard,
	amlGuard *lawenforcement.AntiMoneyLaunderingGuard,
	maxmind *maxminddb.Reader,
) CreateIntentHandler {
	return &SendPrivatePaymentIntentHandler{
		conf:          conf,
		data:          data,
		pusher:        pusher,
		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,
		maxmind:       maxmind,
	}
}

func (h *SendPrivatePaymentIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetSendPrivatePayment()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	exchangeData := typedProtoMetadata.ExchangeData

	// Fetch USD exchange data in a consistent way with the currency server
	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		return errors.Wrap(err, "error getting current usd exchange rate")
	}

	destination, err := common.NewAccountFromProto(typedProtoMetadata.Destination)
	if err != nil {
		return err
	}

	destinationAccountInfo, err := h.data.GetAccountInfoByTokenAddress(ctx, destination.PublicKey().ToBase58())
	if err != nil && err != account.ErrAccountInfoNotFound {
		return err
	}

	var isMicroPayment bool
	requestRecord, err := h.data.GetRequest(ctx, intentRecord.IntentId)
	if err == nil {
		if !requestRecord.RequiresPayment() {
			return newIntentValidationError("request doesn't require payment")
		}

		isMicroPayment = true
		h.cachedPaymentRequestRequest = requestRecord
	} else if err != paymentrequest.ErrPaymentRequestNotFound {
		return err
	}

	intentRecord.IntentType = intent.SendPrivatePayment
	intentRecord.SendPrivatePaymentMetadata = &intent.SendPrivatePaymentMetadata{
		DestinationTokenAccount: destination.PublicKey().ToBase58(),
		Quantity:                exchangeData.Quarks,

		ExchangeCurrency: currency_lib.Code(exchangeData.Currency),
		ExchangeRate:     exchangeData.ExchangeRate,
		NativeAmount:     typedProtoMetadata.ExchangeData.NativeAmount,
		UsdMarketValue:   usdExchangeRecord.Rate * float64(kin.FromQuarks(exchangeData.Quarks)),

		IsWithdrawal:   typedProtoMetadata.IsWithdrawal,
		IsRemoteSend:   typedProtoMetadata.IsRemoteSend,
		IsMicroPayment: isMicroPayment,
		IsTip:          typedProtoMetadata.IsTip,
	}

	if typedProtoMetadata.IsTip {
		if typedProtoMetadata.TippedUser == nil {
			return newIntentValidationError("tipped user metadata is missing")
		}

		intentRecord.SendPrivatePaymentMetadata.TipMetadata = &intent.TipMetadata{
			Platform: typedProtoMetadata.TippedUser.Platform,
			Username: typedProtoMetadata.TippedUser.Username,
		}
	}

	if destinationAccountInfo != nil {
		intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount = destinationAccountInfo.OwnerAccount
	}

	return nil
}

func (h *SendPrivatePaymentIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *SendPrivatePaymentIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	var destinationOwnerAccount, giftCardVaultAccount *common.Account
	var err error

	if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
		destinationOwnerAccount, err = common.NewAccountFromPublicKeyString(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount)
		if err != nil {
			return nil, err
		}
	}

	if intentRecord.SendPrivatePaymentMetadata.IsRemoteSend {
		giftCardVaultAccount, err = common.NewAccountFromPublicKeyString(intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
		if err != nil {
			return nil, err
		}
	}

	return &lockableAccounts{
		DestinationOwner:        destinationOwnerAccount,
		RemoteSendGiftCardVault: giftCardVaultAccount,
	}, nil
}

func (h *SendPrivatePaymentIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := untypedMetadata.GetSendPrivatePayment()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	// todo: need a solution for auto returns
	if intentRecord.SendPrivatePaymentMetadata.IsRemoteSend {
		return newIntentDeniedError("remote send is not supported yet for the vm")
	}
	// todo: need a solution for additional memo containing tipping platform and username in a memo
	if intentRecord.SendPrivatePaymentMetadata.IsTip {
		return newIntentDeniedError("tipping is not supported yet for the vm")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccountsByType, err := common.GetLatestCodeTimelockAccountRecordsForOwner(ctx, h.data, initiatiorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccounts := make([]*common.AccountRecords, 0)
	initiatorAccountsByVault := make(map[string]*common.AccountRecords)
	for _, batchRecords := range initiatorAccountsByType {
		for _, records := range batchRecords {
			initiatorAccounts = append(initiatorAccounts, records)
			initiatorAccountsByVault[records.General.TokenAccount] = records
		}
	}

	//
	// Part 1: Antispam and anti-money laundering guard checks against the phone number
	//

	if !h.conf.disableAntispamChecks.Get(ctx) {
		destination, err := common.NewAccountFromProto(typedMetadata.Destination)
		if err != nil {
			return err
		}

		allow, err := h.antispamGuard.AllowSendPayment(ctx, initiatiorOwnerAccount, false, destination)
		if err != nil {
			return err
		} else if !allow {
			return ErrTooManyPayments
		}
	}

	if !h.conf.disableAmlChecks.Get(ctx) {
		allow, err := h.amlGuard.AllowMoneyMovement(ctx, intentRecord)
		if err != nil {
			return err
		} else if !allow {
			return ErrTransactionLimitExceeded
		}
	}

	//
	// Part 2: Account validation to determine if it's managed by Code
	//

	err = validateAllUserAccountsManagedByCode(ctx, initiatorAccounts)
	if err != nil {
		return err
	}

	//
	// Part 3: Exchange data validation
	//

	if err := validateExchangeDataWithinIntent(ctx, h.data, intentRecord.IntentId, typedMetadata.ExchangeData); err != nil {
		return err
	}

	//
	// Part 4: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 5: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 5: Validate the individual actions
	//

	return h.validateActions(
		ctx,
		initiatiorOwnerAccount,
		initiatorAccountsByType,
		initiatorAccountsByVault,
		intentRecord,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *SendPrivatePaymentIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	intentRecord *intent.Record,
	metadata *transactionpb.SendPrivatePaymentMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	destination, err := common.NewAccountFromProto(metadata.Destination)
	if err != nil {
		return err
	}

	//
	// Part 1: Validate actions match intent metadata
	//

	//
	// Part 1.1: Check destination and fee collection accounts are paid exact quark amount from latest temp outgoing account
	//

	expectedDestinationPayment := int64(metadata.ExchangeData.Quarks)

	// Minimal validation required here since validateFeePayments generically handles
	// most metadata that isn't specific to an intent
	feePayments := simResult.GetFeePayments()
	for _, feePayment := range feePayments {
		expectedDestinationPayment += feePayment.DeltaQuarks
	}

	destinationSimulation, ok := simResult.SimulationsByAccount[destination.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment to destination account %s", destination.PublicKey().ToBase58())
	} else if len(destinationSimulation.Transfers) != 1 {
		return newIntentValidationError("destination account can only receive funds in one action")
	} else if destinationSimulation.Transfers[0].IsPrivate || !destinationSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(destinationSimulation.Transfers[0].Action, "payment sent to destination must be a public withdraw")
	} else if destinationSimulation.GetDeltaQuarks() != expectedDestinationPayment {
		return newActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", expectedDestinationPayment)
	}

	tempOutgoing := initiatorAccountsByType[commonpb.AccountType_TEMPORARY_OUTGOING][0].General.TokenAccount
	tempOutgoingSimulation, ok := simResult.SimulationsByAccount[tempOutgoing]
	if !ok || len(tempOutgoingSimulation.Transfers) == 0 {
		return newIntentValidationErrorf("payment must be sent from temporary outgoing account %s", tempOutgoing)
	}

	lastTransferFromTempOutgoing := tempOutgoingSimulation.Transfers[len(tempOutgoingSimulation.Transfers)-1]
	if lastTransferFromTempOutgoing.Action.Id != destinationSimulation.Transfers[0].Action.Id {
		return newActionValidationErrorf(destinationSimulation.Transfers[0].Action, "destination account must be paid by temporary outgoing account %s", tempOutgoing)
	}

	for _, feePayment := range feePayments {
		if base58.Encode(feePayment.Action.GetFeePayment().Source.Value) != tempOutgoing {
			return newActionValidationErrorf(feePayment.Action, "fee payment must come from temporary outgoing account %s", tempOutgoing)
		}
	}

	if tempOutgoingSimulation.GetDeltaQuarks() != 0 {
		return newIntentValidationErrorf("must fund temporary outgoing account with %d quarks", metadata.ExchangeData.Quarks)
	}

	//
	// Part 1.2: Check destination account based on withdrawal, remote send and tip flags
	//

	// Invalid combination of various payment flags
	if metadata.IsRemoteSend && metadata.IsWithdrawal {
		return newIntentValidationError("withdrawal and remote send flags cannot both be true set at the same time")
	}
	if metadata.IsRemoteSend && intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		return newIntentValidationError("remote send cannot be used for micro payments")
	}
	if metadata.IsRemoteSend && metadata.IsTip {
		return newIntentValidationError("remote send cannot be used for tips")
	}
	if metadata.IsTip && intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		return newIntentValidationError("tips cannot be used for micro payments")
	}
	if metadata.IsTip && !metadata.IsWithdrawal {
		return newIntentValidationError("tips must be a private withdrawal")
	}

	// Note: Assumes account info only stores Code user accounts
	destinationAccountInfo, err := h.data.GetAccountInfoByTokenAddress(ctx, destination.PublicKey().ToBase58())
	switch err {
	case nil:
		// The remote send gift card cannot exist. It must be created as part of this intent.
		if metadata.IsRemoteSend {
			return newIntentValidationError("remote send must be to a brand new gift card account")
		}

		if metadata.IsTip && metadata.TippedUser == nil {
			return newIntentValidationError("tipped user metadata is missing")
		}
		if metadata.IsTip && destinationAccountInfo.AccountType != commonpb.AccountType_PRIMARY {
			return newIntentValidationError("destination account must be a primary account")
		}
		if metadata.IsTip {
			err = validateTipDestination(ctx, h.data, metadata.TippedUser, destination)
			if err != nil {
				return err
			}
		}

		// Code->Code withdrawals must be sent to a deposit account. We allow the
		// same owner, since the user might be funding a public withdrawal.
		//
		// Note: For relationship accounts used in payment requests, the relationship
		//       has already been validated by the messaging service.
		if metadata.IsWithdrawal && destinationAccountInfo.AccountType != commonpb.AccountType_PRIMARY && destinationAccountInfo.AccountType != commonpb.AccountType_RELATIONSHIP {
			return newIntentValidationError("destination account must be a deposit account")
		}

		if !metadata.IsWithdrawal {
			// Code->Code payments must be sent to a temporary incoming account
			if destinationAccountInfo.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
				return newIntentValidationError("destination account must be a temporary incoming account")
			}

			// That temporary incoming account must be the latest one
			latestTempIncomingAccountInfo, err := h.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, destinationAccountInfo.OwnerAccount, commonpb.AccountType_TEMPORARY_INCOMING)
			if err != nil {
				return err
			} else if latestTempIncomingAccountInfo.TokenAccount != destinationAccountInfo.TokenAccount {
				// Important Note: Do not leak the account address and break privacy!
				return newStaleStateError("destination is not the latest temporary incoming account")
			}

			// The temporary incoming account has limited usage
			err = validateMinimalTempIncomingAccountUsage(ctx, h.data, destinationAccountInfo)
			if err != nil {
				return err
			}

			// And that payment cannot be done to the same owner
			if destinationAccountInfo.OwnerAccount == initiatorOwnerAccount.PublicKey().ToBase58() {
				return newIntentValidationError("payments within the same owner are not allowed")
			}
		}
	case account.ErrAccountInfoNotFound:
		// There are two cases:
		//  1. An external token account that must be validated to exist.
		//  2. A remote send gift card that will be created as part of this intent.
		if metadata.IsWithdrawal {
			if !h.conf.disableBlockchainChecks.Get(ctx) {
				err = validateExternalKinTokenAccountWithinIntent(ctx, h.data, destination)
				if err != nil {
					return err
				}
			}
		} else if metadata.IsRemoteSend {
			// No validation needed here. Open validation is handled later.
		} else {
			// The client is trying a Code->Code payment since withdrawal and remote
			// send flags are both false. The temporary incoming account doesn't exist.
			return newIntentValidationError("destination account must be a temporary incoming account")
		}
	default:
		return err
	}

	//
	// Part 1.3: Validate destination account matches payment request, if it exists
	//

	if h.cachedPaymentRequestRequest != nil && *h.cachedPaymentRequestRequest.DestinationTokenAccount != destination.PublicKey().ToBase58() {
		return newIntentValidationErrorf("payment has a request to destination %s", *h.cachedPaymentRequestRequest.DestinationTokenAccount)
	}

	//
	// Part 2: Validate actions that open/close accounts
	//

	openedAccounts := simResult.GetOpenedAccounts()
	if metadata.IsRemoteSend {
		// There are two opened accounts, one for the gift card and one for the temporary outgoing account
		if len(openedAccounts) != 2 {
			return newIntentValidationError("must open two accounts")
		}

		err = validateGiftCardAccountOpened(
			initiatorOwnerAccount,
			initiatorAccountsByType,
			destination,
			actions,
		)
		if err != nil {
			return err
		}
	} else {
		// There's one opened account, and it must be a new temporary outgoing account.
		if len(openedAccounts) != 1 {
			return newIntentValidationError("must open one account")
		}
	}

	err = validateNextTemporaryAccountOpened(
		commonpb.AccountType_TEMPORARY_OUTGOING,
		initiatorOwnerAccount,
		initiatorAccountsByType,
		actions,
	)
	if err != nil {
		return err
	}

	// There's one closed account, and it must be the latest temporary outgoing account.
	closedAccounts := simResult.GetClosedAccounts()
	if len(closedAccounts) != 1 {
		return newIntentValidationError("must close one account")
	} else if closedAccounts[0].TokenAccount.PublicKey().ToBase58() != tempOutgoing {
		return newActionValidationError(closedAccounts[0].CloseAction, "must close latest temporary outgoing account")
	}

	//
	// Part 3: Validate actions that move money
	//

	err = validateMoneyMovementActionCount(actions)
	if err != nil {
		return err
	}

	for _, simulation := range simResult.SimulationsByAccount {
		if len(simulation.Transfers) == 0 {
			continue
		}

		// Destination account transfers are already validated during intent
		// metadata validation
		if simulation.TokenAccount.PublicKey().ToBase58() == destination.PublicKey().ToBase58() {
			continue
		}

		// By previous validation, we know the only opened account is the new temp
		// outgoing account
		if simulation.Opened {
			return newActionValidationError(simulation.Transfers[0].Action, "new temporary outgoing account cannot send/receive kin")
		}

		// Every account going forward must be part of the initiator owner's latest
		// account set.
		accountRecords, ok := initiatorAccountsByVault[simulation.TokenAccount.PublicKey().ToBase58()]
		if !ok {
			usedAs := "source"
			if simulation.Transfers[0].DeltaQuarks > 0 {
				usedAs = "destination"
			}
			return newActionValidationErrorf(simulation.Transfers[0].Action, "%s is not a latest owned account", usedAs)
		}

		// Enforce how each account can send/receive Kin
		switch accountRecords.General.AccountType {
		case commonpb.AccountType_PRIMARY:
			return newActionValidationError(simulation.Transfers[0].Action, "primary account cannot send/receive kin")
		case commonpb.AccountType_TEMPORARY_INCOMING:
			return newActionValidationError(simulation.Transfers[0].Action, "temporary incoming account cannot send/receive kin")
		case commonpb.AccountType_TEMPORARY_OUTGOING:
			expectedPublicTransfers := 1
			if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
				// Code fee, plus any additional configured fee takers
				expectedPublicTransfers += len(h.cachedPaymentRequestRequest.Fees) + 1
			}
			if simulation.CountPublicTransfers() != expectedPublicTransfers {
				return newIntentValidationErrorf("temporary outgoing account can only have %d public transfers", expectedPublicTransfers)
			}

			if simulation.CountWithdrawals() != 1 {
				return newIntentValidationError("temporary outgoing account can only have one public withdraw")
			}

			if simulation.CountOutgoingTransfers() != expectedPublicTransfers {
				return newIntentValidationErrorf("temporary outgoing account can only send kin %d times", expectedPublicTransfers)
			}
		case commonpb.AccountType_BUCKET_1_KIN,
			commonpb.AccountType_BUCKET_10_KIN,
			commonpb.AccountType_BUCKET_100_KIN,
			commonpb.AccountType_BUCKET_1_000_KIN,
			commonpb.AccountType_BUCKET_10_000_KIN,
			commonpb.AccountType_BUCKET_100_000_KIN,
			commonpb.AccountType_BUCKET_1_000_000_KIN:

			err = validateBucketAccountSimulation(accountRecords.General.AccountType, simulation)
			if err != nil {
				return err
			}
		default:
			return errors.New("unhandled account type")
		}
	}

	err = validateMoneyMovementActionUserAccounts(intent.SendPrivatePayment, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	//
	// Part 4: Validate there are no update actions
	//

	return validateNoUpgradeActions(actions)
}

func (h *SendPrivatePaymentIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	var eventRecord *event.Record
	if !intentRecord.SendPrivatePaymentMetadata.IsRemoteSend && !intentRecord.SendPrivatePaymentMetadata.IsWithdrawal && !intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		// todo: Collect additional destination info on private receive
		eventRecord = &event.Record{
			EventId:   intentRecord.IntentId,
			EventType: event.InPersonGrab,

			SourceCodeAccount:      intentRecord.InitiatorOwnerAccount,
			DestinationCodeAccount: &intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount,

			SourceIdentity: *intentRecord.InitiatorPhoneNumber,

			UsdValue: &intentRecord.SendPrivatePaymentMetadata.UsdMarketValue,

			SpamConfidence: 0,

			CreatedAt: time.Now(),
		}
	} else if intentRecord.SendPrivatePaymentMetadata.IsRemoteSend {
		eventRecord = &event.Record{
			EventId:   intentRecord.IntentId,
			EventType: event.RemoteSend,

			SourceCodeAccount: intentRecord.InitiatorOwnerAccount,

			SourceIdentity: *intentRecord.InitiatorPhoneNumber,

			UsdValue: &intentRecord.SendPrivatePaymentMetadata.UsdMarketValue,

			SpamConfidence: 0,

			CreatedAt: time.Now(),
		}
	} else if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		eventRecord = &event.Record{
			EventId:   intentRecord.IntentId,
			EventType: event.MicroPayment,

			SourceCodeAccount: intentRecord.InitiatorOwnerAccount,

			SourceIdentity: *intentRecord.InitiatorPhoneNumber,

			UsdValue: &intentRecord.SendPrivatePaymentMetadata.UsdMarketValue,

			SpamConfidence: 0,

			CreatedAt: time.Now(),
		}

		if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
			eventRecord.DestinationCodeAccount = &intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount
		} else {
			eventRecord.ExternalTokenAccount = &intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount
		}
	}

	if eventRecord != nil {
		event_util.InjectClientDetails(ctx, h.maxmind, eventRecord, true)

		if eventRecord.DestinationCodeAccount != nil {
			destinationVerificationRecord, err := h.data.GetLatestPhoneVerificationForAccount(ctx, *eventRecord.DestinationCodeAccount)
			if err != nil {
				return err
			}
			eventRecord.DestinationIdentity = &destinationVerificationRecord.PhoneNumber
		}

		err := h.data.SaveEvent(ctx, eventRecord)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *SendPrivatePaymentIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

type ReceivePaymentsPrivatelyIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
	amlGuard      *lawenforcement.AntiMoneyLaunderingGuard
}

func NewReceivePaymentsPrivatelyIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard, amlGuard *lawenforcement.AntiMoneyLaunderingGuard) CreateIntentHandler {
	return &ReceivePaymentsPrivatelyIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,
	}
}

func (h *ReceivePaymentsPrivatelyIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetReceivePaymentsPrivately()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		return errors.Wrap(err, "error getting current usd exchange rate")
	}

	intentRecord.IntentType = intent.ReceivePaymentsPrivately
	intentRecord.ReceivePaymentsPrivatelyMetadata = &intent.ReceivePaymentsPrivatelyMetadata{
		Source:    base58.Encode(typedProtoMetadata.Source.Value),
		Quantity:  typedProtoMetadata.Quarks,
		IsDeposit: typedProtoMetadata.IsDeposit,

		UsdMarketValue: usdExchangeRecord.Rate * float64(kin.FromQuarks(typedProtoMetadata.Quarks)),
	}

	return nil
}

func (h *ReceivePaymentsPrivatelyIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *ReceivePaymentsPrivatelyIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	return &lockableAccounts{}, nil
}

func (h *ReceivePaymentsPrivatelyIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := untypedMetadata.GetReceivePaymentsPrivately()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccountsByType, err := common.GetLatestCodeTimelockAccountRecordsForOwner(ctx, h.data, initiatiorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccounts := make([]*common.AccountRecords, 0)
	initiatorAccountsByVault := make(map[string]*common.AccountRecords)
	for _, batchRecords := range initiatorAccountsByType {
		for _, records := range batchRecords {
			initiatorAccounts = append(initiatorAccounts, records)
			initiatorAccountsByVault[records.General.TokenAccount] = records
		}
	}

	//
	// Part 1: Intent ID validation
	//

	err = validateIntentIdIsNotRequest(ctx, h.data, intentRecord.IntentId)
	if err != nil {
		return err
	}

	//
	// Part 2: Antispam and anti-money laundering guard checks against the phone number
	//
	if !h.conf.disableAntispamChecks.Get(ctx) {
		allow, err := h.antispamGuard.AllowReceivePayments(ctx, initiatiorOwnerAccount, false)
		if err != nil {
			return err
		} else if !allow {
			return ErrTooManyPayments
		}
	}

	if !h.conf.disableAmlChecks.Get(ctx) {
		allow, err := h.amlGuard.AllowMoneyMovement(ctx, intentRecord)
		if err != nil {
			return err
		} else if !allow {
			return ErrTransactionLimitExceeded
		}
	}

	//
	// Part 3: Account validation to determine if it's managed by Code
	//

	err = validateAllUserAccountsManagedByCode(ctx, initiatorAccounts)
	if err != nil {
		return err
	}

	//
	// Part 4: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 5: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 6: Validate the individual actions
	//

	return h.validateActions(
		ctx,
		initiatiorOwnerAccount,
		initiatorAccountsByType,
		initiatorAccountsByVault,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *ReceivePaymentsPrivatelyIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	metadata *transactionpb.ReceivePaymentsPrivatelyMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	source, err := common.NewAccountFromProto(metadata.Source)
	if err != nil {
		return err
	}

	//
	// Part 1: Validate actions match intent metadata
	//

	//
	// Part 1.1: Check exact quark amount is received from the source account
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok || len(sourceSimulation.Transfers) == 0 {
		return newIntentValidationError("must receive payments from source account")
	} else if sourceSimulation.GetDeltaQuarks() != -int64(metadata.Quarks) {
		return newIntentValidationErrorf("must receive %d quarks from source account", metadata.Quarks)
	}

	//
	// Part 1.2: Check source account type based on deposit flag
	//

	sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationError("source is not a latest owned account")
	} else if metadata.IsDeposit && sourceAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY && sourceAccountInfo.General.AccountType != commonpb.AccountType_RELATIONSHIP {
		return newIntentValidationError("must receive from a deposit account")
	} else if !metadata.IsDeposit && sourceAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
		return newIntentValidationError("must receive from latest temporary incoming account")
	}

	//
	// Part 2: Validate actions that open/close accounts
	//

	openedAccounts := simResult.GetOpenedAccounts()
	closedAccounts := simResult.GetClosedAccounts()
	if metadata.IsDeposit {
		if len(openedAccounts) > 0 {
			return newActionValidationError(openedAccounts[0].OpenAction, "cannot open any account")
		}

		if len(closedAccounts) > 0 {
			return newActionValidationError(closedAccounts[0].CloseAction, "cannot close any account")
		}
	} else {

		// There's one opened account, and it must be the new temporary incoming account.
		if len(openedAccounts) != 1 {
			return newIntentValidationError("must open one account")
		}

		err = validateNextTemporaryAccountOpened(
			commonpb.AccountType_TEMPORARY_INCOMING,
			initiatorOwnerAccount,
			initiatorAccountsByType,
			actions,
		)
		if err != nil {
			return err
		}

		// No accounts are closed, the latest temporary incoming account is simply
		// compressed after used.
		closedAccounts := simResult.GetClosedAccounts()
		if len(closedAccounts) != 0 {
			return newIntentValidationError("cannot close any account")
		}
	}

	//
	// Part 3: Validate actions that move money
	//

	err = validateMoneyMovementActionCount(actions)
	if err != nil {
		return err
	}

	for _, simulation := range simResult.SimulationsByAccount {
		if len(simulation.Transfers) == 0 {
			continue
		}

		// By previous validation, we know the only opened account is the new temp
		// incoming account
		if simulation.Opened {
			return newActionValidationError(simulation.Transfers[0].Action, "new temporary incoming account cannot send/receive kin")
		}

		// Every account going forward must be part of the initiator owner's latest
		// account set.
		accountRecords, ok := initiatorAccountsByVault[simulation.TokenAccount.PublicKey().ToBase58()]
		if !ok {
			usedAs := "source"
			if simulation.Transfers[0].DeltaQuarks > 0 {
				usedAs = "destination"
			}
			return newActionValidationErrorf(simulation.Transfers[0].Action, "%s is not a latest owned account", usedAs)
		}

		// Enforce how each account can send/receive Kin
		switch accountRecords.General.AccountType {
		case commonpb.AccountType_TEMPORARY_OUTGOING:
			return newActionValidationError(simulation.Transfers[0].Action, "temporary outgoing account cannot send/receive kin")
		case commonpb.AccountType_PRIMARY, commonpb.AccountType_RELATIONSHIP:
			if !metadata.IsDeposit {
				return newActionValidationError(simulation.Transfers[0].Action, "deposit account cannot send/receive kin")
			}

			if metadata.IsDeposit {
				incomingTransfers := simulation.GetIncomingTransfers()
				if len(incomingTransfers) > 0 {
					return newActionValidationError(incomingTransfers[0].Action, "deposit account cannot receive kin")
				}

				publicTransfers := simulation.GetPublicTransfers()
				if len(publicTransfers) > 0 {
					return newActionValidationError(publicTransfers[0].Action, "receipt of kin must be private")
				}

				withdraws := simulation.GetWithdraws()
				if len(withdraws) > 0 {
					return newActionValidationError(withdraws[0].Action, "receipt of kin cannot be done with a withdraw")
				}
			}
		case commonpb.AccountType_TEMPORARY_INCOMING:
			if metadata.IsDeposit {
				return newActionValidationError(simulation.Transfers[0].Action, "temporary incoming account cannot send/receive kin")
			}

			if !metadata.IsDeposit {
				incomingTransfers := simulation.GetIncomingTransfers()
				if len(incomingTransfers) > 0 {
					return newActionValidationError(incomingTransfers[0].Action, "temporary incoming account cannot receive kin")
				}

				publicTransfers := simulation.GetPublicTransfers()
				if len(publicTransfers) > 0 {
					return newActionValidationError(publicTransfers[0].Action, "receipt of kin must be private")
				}

				withdraws := simulation.GetWithdraws()
				if len(withdraws) > 0 {
					return newActionValidationError(withdraws[0].Action, "receipt of kin cannot be done with a withdraw")
				}
			}
		case commonpb.AccountType_BUCKET_1_KIN,
			commonpb.AccountType_BUCKET_10_KIN,
			commonpb.AccountType_BUCKET_100_KIN,
			commonpb.AccountType_BUCKET_1_000_KIN,
			commonpb.AccountType_BUCKET_10_000_KIN,
			commonpb.AccountType_BUCKET_100_000_KIN,
			commonpb.AccountType_BUCKET_1_000_000_KIN:

			err = validateBucketAccountSimulation(accountRecords.General.AccountType, simulation)
			if err != nil {
				return err
			}
		default:
			return errors.New("unhandled account type")
		}
	}

	err = validateMoneyMovementActionUserAccounts(intent.ReceivePaymentsPrivately, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	//
	// Part 4: Validate there are no upgrade actions
	//

	return validateNoUpgradeActions(actions)
}

func (h *ReceivePaymentsPrivatelyIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func (h *ReceivePaymentsPrivatelyIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

/*
type UpgradePrivacyIntentHandler struct {
	conf                 *conf
	data                 code_data.Provider
	cachedUpgradeTargets map[uint32]*privacyUpgradeCandidate
}

func NewUpgradePrivacyIntentHandler(conf *conf, data code_data.Provider) UpdateIntentHandler {
	return &UpgradePrivacyIntentHandler{
		conf: conf,
		data: data,
	}
}

func (h *UpgradePrivacyIntentHandler) AllowUpdate(ctx context.Context, existingIntent *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
	if untypedMetadata.GetUpgradePrivacy() == nil {
		return errors.New("unexpected metadata proto message")
	}

	cachedUpgradeTargets := make(map[uint32]*privacyUpgradeCandidate)
	for _, untypedAction := range actions {
		var actionId uint32
		switch typedAction := untypedAction.Type.(type) {
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			actionId = typedAction.PermanentPrivacyUpgrade.ActionId
			_, ok := cachedUpgradeTargets[actionId]
			if ok {
				return newActionValidationError(untypedAction, "duplicate upgrade action detected")
			}
		default:
			return newActionValidationError(untypedAction, "all actions must be to upgrade private transactions")
		}

		cachedUpgradeTarget, err := selectCandidateForPrivacyUpgrade(ctx, h.data, existingIntent.IntentId, actionId)
		switch err {
		case nil:
		case ErrPrivacyAlreadyUpgraded:
			return newActionWithStaleStateError(untypedAction, err.Error())
		case ErrInvalidActionToUpgrade, ErrPrivacyUpgradeMissed, ErrWaitForNextBlock:
			return newActionValidationError(untypedAction, err.Error())
		default:
			return err
		}
		cachedUpgradeTargets[actionId] = cachedUpgradeTarget
	}
	h.cachedUpgradeTargets = cachedUpgradeTargets

	return nil
}

func (h *UpgradePrivacyIntentHandler) GetCachedUpgradeTarget(protoAction *transactionpb.PermanentPrivacyUpgradeAction) (*privacyUpgradeCandidate, bool) {
	upgradeTo, ok := h.cachedUpgradeTargets[protoAction.ActionId]
	return upgradeTo, ok
}
*/

type SendPublicPaymentIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	pusher        push_lib.Provider
	antispamGuard *antispam.Guard
	maxmind       *maxminddb.Reader
}

func NewSendPublicPaymentIntentHandler(
	conf *conf,
	data code_data.Provider,
	pusher push_lib.Provider,
	antispamGuard *antispam.Guard,
	maxmind *maxminddb.Reader,
) CreateIntentHandler {
	return &SendPublicPaymentIntentHandler{
		conf:          conf,
		data:          data,
		pusher:        pusher,
		antispamGuard: antispamGuard,
		maxmind:       maxmind,
	}
}

func (h *SendPublicPaymentIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetSendPublicPayment()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	exchangeData := typedProtoMetadata.ExchangeData

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		return errors.Wrap(err, "error getting current usd exchange rate")
	}

	destination, err := common.NewAccountFromProto(typedProtoMetadata.Destination)
	if err != nil {
		return err
	}

	destinationAccountInfo, err := h.data.GetAccountInfoByTokenAddress(ctx, destination.PublicKey().ToBase58())
	if err != nil && err != account.ErrAccountInfoNotFound {
		return err
	}

	intentRecord.IntentType = intent.SendPublicPayment
	intentRecord.SendPublicPaymentMetadata = &intent.SendPublicPaymentMetadata{
		DestinationTokenAccount: destination.PublicKey().ToBase58(),
		Quantity:                exchangeData.Quarks,

		ExchangeCurrency: currency_lib.Code(exchangeData.Currency),
		ExchangeRate:     exchangeData.ExchangeRate,
		NativeAmount:     typedProtoMetadata.ExchangeData.NativeAmount,
		UsdMarketValue:   usdExchangeRecord.Rate * float64(kin.FromQuarks(exchangeData.Quarks)),

		IsWithdrawal: typedProtoMetadata.IsWithdrawal,
	}

	if destinationAccountInfo != nil {
		intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount = destinationAccountInfo.OwnerAccount
	}

	return nil
}

func (h *SendPublicPaymentIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *SendPublicPaymentIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	if len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) == 0 {
		return &lockableAccounts{}, nil
	}

	destinationOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
	if err != nil {
		return nil, err
	}

	return &lockableAccounts{
		DestinationOwner: destinationOwnerAccount,
	}, nil
}

func (h *SendPublicPaymentIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := untypedMetadata.GetSendPublicPayment()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccountsByType, err := common.GetLatestCodeTimelockAccountRecordsForOwner(ctx, h.data, initiatiorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccounts := make([]*common.AccountRecords, 0)
	initiatorAccountsByVault := make(map[string]*common.AccountRecords)
	for _, batchRecords := range initiatorAccountsByType {
		for _, records := range batchRecords {
			initiatorAccounts = append(initiatorAccounts, records)
			initiatorAccountsByVault[records.General.TokenAccount] = records
		}
	}

	//
	// Part 1: Intent ID validation
	//

	err = validateIntentIdIsNotRequest(ctx, h.data, intentRecord.IntentId)
	if err != nil {
		return err
	}

	//
	// Part 2: Antispam guard checks against the phone number
	//

	if !h.conf.disableAntispamChecks.Get(ctx) {
		destination, err := common.NewAccountFromProto(typedMetadata.Destination)
		if err != nil {
			return err
		}

		allow, err := h.antispamGuard.AllowSendPayment(ctx, initiatiorOwnerAccount, true, destination)
		if err != nil {
			return err
		} else if !allow {
			return ErrTooManyPayments
		}
	}

	//
	// Part 3: Account validation to determine if it's managed by Code
	//

	err = validateAllUserAccountsManagedByCode(ctx, initiatorAccounts)
	if err != nil {
		return err
	}

	//
	// Part 4: Exchange data validation
	//

	if err := validateExchangeDataWithinIntent(ctx, h.data, intentRecord.IntentId, typedMetadata.ExchangeData); err != nil {
		return err
	}

	//
	// Part 5: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 6: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 7: Validate the individual actions
	//

	return h.validateActions(
		ctx,
		initiatiorOwnerAccount,
		initiatorAccountsByType,
		initiatorAccountsByVault,
		intentRecord,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *SendPublicPaymentIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	intentRecord *intent.Record,
	metadata *transactionpb.SendPublicPaymentMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	if len(actions) != 1 {
		return newIntentValidationError("expected 1 action")
	}

	var source *common.Account
	var err error
	if metadata.Source != nil {
		source, err = common.NewAccountFromProto(metadata.Source)
		if err != nil {
			return err
		}
	} else {
		// Backwards compat for old clients using metadata without source. It was
		// always assumed to be from the primary account
		source, err = common.NewAccountFromPublicKeyString(initiatorAccountsByType[commonpb.AccountType_PRIMARY][0].General.TokenAccount)
		if err != nil {
			return err
		}
	}

	destination, err := common.NewAccountFromProto(metadata.Destination)
	if err != nil {
		return err
	}

	// Part 1: Check the source and destination accounts are valid

	destinationAccountInfo, err := h.data.GetAccountInfoByTokenAddress(ctx, destination.PublicKey().ToBase58())
	switch err {
	case nil:
		// Code->Code public withdraws must be done against other deposit accounts
		if metadata.IsWithdrawal && destinationAccountInfo.AccountType != commonpb.AccountType_PRIMARY && destinationAccountInfo.AccountType != commonpb.AccountType_RELATIONSHIP {
			return newIntentValidationError("destination account must be a deposit account")
		}

		// And the destination cannot be the source of funds, since that results in a no-op
		if source.PublicKey().ToBase58() == destinationAccountInfo.TokenAccount {
			return newIntentValidationError("payment is a no-op")
		}
	case account.ErrAccountInfoNotFound:
		// Check whether the destination account is a Kin token account that's
		// been created on the blockchain.
		if !h.conf.disableBlockchainChecks.Get(ctx) {
			err = validateExternalKinTokenAccountWithinIntent(ctx, h.data, destination)
			if err != nil {
				return err
			}
		}
	default:
		return err
	}

	sourceAccountRecords, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
	if !ok || (sourceAccountRecords.General.AccountType != commonpb.AccountType_PRIMARY && sourceAccountRecords.General.AccountType != commonpb.AccountType_RELATIONSHIP) {
		return newIntentValidationError("source account must be a deposit account")
	}

	//
	// Part 2: Validate actions match intent metadata
	//

	//
	// Part 2.1: Check destination account is paid exact quark amount from the deposit account
	//

	destinationSimulation, ok := simResult.SimulationsByAccount[destination.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment to destination account %s", destination.PublicKey().ToBase58())
	} else if destinationSimulation.Transfers[0].IsPrivate || destinationSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(destinationSimulation.Transfers[0].Action, "payment sent to destination must be a public transfer")
	} else if destinationSimulation.GetDeltaQuarks() != int64(metadata.ExchangeData.Quarks) {
		return newActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", metadata.ExchangeData.Quarks)
	}

	//
	// Part 2.2: Check that the user's deposit account was used as the source of funds
	//           as specified in the metadata
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment from source account %s", source.PublicKey().ToBase58())
	} else if sourceSimulation.GetDeltaQuarks() != -int64(metadata.ExchangeData.Quarks) {
		return newActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must send %d quarks from source account", metadata.ExchangeData.Quarks)
	}

	// Part 3: Generic validation of actions that move money

	err = validateMoneyMovementActionUserAccounts(intent.SendPublicPayment, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	// Part 4: Sanity check no open and closed accounts

	if len(simResult.GetOpenedAccounts()) > 0 {
		return newIntentValidationError("cannot open any account")
	}

	if len(simResult.GetClosedAccounts()) > 0 {
		return newIntentValidationError("cannot close any account")
	}

	return nil
}

func (h *SendPublicPaymentIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	if intentRecord.SendPublicPaymentMetadata.IsWithdrawal && len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) == 0 {
		eventRecord := &event.Record{
			EventId:   intentRecord.IntentId,
			EventType: event.Withdrawal,

			SourceCodeAccount:    intentRecord.InitiatorOwnerAccount,
			ExternalTokenAccount: &intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount,

			SourceIdentity: *intentRecord.InitiatorPhoneNumber,

			UsdValue: &intentRecord.SendPublicPaymentMetadata.UsdMarketValue,

			SpamConfidence: 0,

			CreatedAt: time.Now(),
		}
		event_util.InjectClientDetails(ctx, h.maxmind, eventRecord, true)

		err := h.data.SaveEvent(ctx, eventRecord)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *SendPublicPaymentIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

type ReceivePaymentsPubliclyIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
	maxmind       *maxminddb.Reader

	cachedGiftCardIssuedIntentRecord *intent.Record
}

func NewReceivePaymentsPubliclyIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard, maxmind *maxminddb.Reader) CreateIntentHandler {
	return &ReceivePaymentsPubliclyIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		maxmind:       maxmind,
	}
}

func (h *ReceivePaymentsPubliclyIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetReceivePaymentsPublicly()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	giftCardVault, err := common.NewAccountFromPublicKeyBytes(typedProtoMetadata.Source.Value)
	if err != nil {
		return err
	}

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		return errors.Wrap(err, "error getting current usd exchange rate")
	}

	// This is an optimization for payment history. Original fiat amounts are not
	// easily linked due to the nature of gift cards and the remote send flow. We
	// fetch this metadata up front so we don't need to do it every time in history.
	giftCardIssuedIntentRecord, err := h.data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVault.PublicKey().ToBase58())
	if err == intent.ErrIntentNotFound {
		return newIntentValidationError("source is not a remote send gift card")
	} else if err != nil {
		return err
	}
	h.cachedGiftCardIssuedIntentRecord = giftCardIssuedIntentRecord

	intentRecord.IntentType = intent.ReceivePaymentsPublicly
	intentRecord.ReceivePaymentsPubliclyMetadata = &intent.ReceivePaymentsPubliclyMetadata{
		Source:                  giftCardVault.PublicKey().ToBase58(),
		Quantity:                typedProtoMetadata.Quarks,
		IsRemoteSend:            typedProtoMetadata.IsRemoteSend,
		IsReturned:              false,
		IsIssuerVoidingGiftCard: typedProtoMetadata.IsIssuerVoidingGiftCard,

		OriginalExchangeCurrency: giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.ExchangeCurrency,
		OriginalExchangeRate:     giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.ExchangeRate,
		OriginalNativeAmount:     giftCardIssuedIntentRecord.SendPrivatePaymentMetadata.NativeAmount,

		UsdMarketValue: usdExchangeRecord.Rate * float64(kin.FromQuarks(typedProtoMetadata.Quarks)),
	}

	if intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard && intentRecord.InitiatorOwnerAccount != giftCardIssuedIntentRecord.InitiatorOwnerAccount {
		return newIntentValidationError("only the issuer can void the gift card")
	}

	return nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	if !intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
		return &lockableAccounts{}, nil
	}

	giftCardVaultAccount, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
	if err != nil {
		return nil, err
	}

	return &lockableAccounts{
		RemoteSendGiftCardVault: giftCardVaultAccount,
	}, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := untypedMetadata.GetReceivePaymentsPublicly()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	if !typedMetadata.IsRemoteSend {
		return newIntentValidationError("only remote send is supported")
	}

	if typedMetadata.ExchangeData != nil {
		return newIntentValidationError("exchange data cannot be set")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	giftCardVaultAccount, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
	if err != nil {
		return err
	}

	initiatorAccountsByType, err := common.GetLatestCodeTimelockAccountRecordsForOwner(ctx, h.data, initiatiorOwnerAccount)
	if err != nil {
		return err
	}

	initiatorAccounts := make([]*common.AccountRecords, 0)
	initiatorAccountsByVault := make(map[string]*common.AccountRecords)
	for _, batchRecords := range initiatorAccountsByType {
		for _, records := range batchRecords {
			initiatorAccounts = append(initiatorAccounts, records)
			initiatorAccountsByVault[records.General.TokenAccount] = records
		}
	}

	//
	// Part 1: Intent ID validation
	//

	err = validateIntentIdIsNotRequest(ctx, h.data, intentRecord.IntentId)
	if err != nil {
		return err
	}

	//
	// Part 2: Antispam guard checks against the phone number
	//
	if !h.conf.disableAntispamChecks.Get(ctx) {
		allow, err := h.antispamGuard.AllowReceivePayments(ctx, initiatiorOwnerAccount, true)
		if err != nil {
			return err
		} else if !allow {
			return ErrTooManyPayments
		}
	}

	//
	// Part 3: User account validation to determine if it's managed by Code
	//

	err = validateAllUserAccountsManagedByCode(ctx, initiatorAccounts)
	if err != nil {
		return err
	}

	//
	// Part 4: Gift card account validation
	//

	err = validateClaimedGiftCard(ctx, h.data, giftCardVaultAccount, typedMetadata.Quarks)
	if err != nil {
		return err
	}

	//
	// Part 5: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 6: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 7: Validate the individual actions
	//

	return h.validateActions(
		ctx,
		initiatiorOwnerAccount,
		initiatorAccountsByType,
		initiatorAccountsByVault,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *ReceivePaymentsPubliclyIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	metadata *transactionpb.ReceivePaymentsPubliclyMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	if len(actions) != 1 {
		return newIntentValidationError("expected 1 action")
	}

	//
	// Part 1: Validate source and destination accounts are valid to use
	//

	// Note: Already validated to be a claimable gift card elsewhere
	source, err := common.NewAccountFromProto(metadata.Source)
	if err != nil {
		return err
	}

	// The destination account must be the latest temporary incoming account
	destinationAccountInfo := initiatorAccountsByType[commonpb.AccountType_TEMPORARY_INCOMING][0].General
	destinationSimulation, ok := simResult.SimulationsByAccount[destinationAccountInfo.TokenAccount]
	if !ok {
		return newActionValidationError(actions[0], "must send payment to latest temp incoming account")
	}

	// And that temporary incoming account has limited usage
	err = validateMinimalTempIncomingAccountUsage(ctx, h.data, destinationAccountInfo)
	if err != nil {
		return err
	}

	//
	// Part 2: Validate actions match intent
	//

	//
	// Part 2.1: Check source account pays exact quark amount to destination in a public withdraw
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationError("must receive payments from source account")
	} else if sourceSimulation.GetDeltaQuarks() != -int64(metadata.Quarks) {
		return newActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must receive %d quarks from source account", metadata.Quarks)
	} else if sourceSimulation.Transfers[0].IsPrivate || !sourceSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(sourceSimulation.Transfers[0].Action, "transfer must be a public withdraw")
	}

	//
	// Part 2.2: Check destination account is paid exact quark amount from source account in a public withdraw
	//

	if destinationSimulation.GetDeltaQuarks() != int64(metadata.Quarks) {
		return newActionValidationErrorf(actions[0], "must receive %d quarks to temp incoming account", metadata.Quarks)
	} else if destinationSimulation.Transfers[0].IsPrivate || !destinationSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(sourceSimulation.Transfers[0].Action, "transfer must be a public withdraw")
	}

	//
	// Part 3: Validate accounts that are opened and closed
	//

	if len(simResult.GetOpenedAccounts()) > 0 {
		return newIntentValidationError("cannot open any account")
	}

	closedAccounts := simResult.GetClosedAccounts()
	if len(closedAccounts) != 1 {
		return newIntentValidationError("must close 1 account")
	} else if closedAccounts[0].TokenAccount.PublicKey().ToBase58() != source.PublicKey().ToBase58() {
		return newActionValidationError(actions[0], "must close source account")
	}

	//
	// Part 4: Generic validation of actions that move money
	//

	return validateMoneyMovementActionUserAccounts(intent.ReceivePaymentsPublicly, initiatorAccountsByVault, actions)
}

func (h *ReceivePaymentsPubliclyIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	if intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
		eventRecord, err := h.data.GetEvent(ctx, h.cachedGiftCardIssuedIntentRecord.IntentId)
		if err == nil {
			eventRecord.DestinationCodeAccount = &intentRecord.InitiatorOwnerAccount
			eventRecord.DestinationIdentity = pointer.StringCopy(intentRecord.InitiatorPhoneNumber)
			event_util.InjectClientDetails(ctx, h.maxmind, eventRecord, false) // Will be AWS if desktop

			err = h.data.SaveEvent(ctx, eventRecord)
			if err != nil {
				return err
			}
		} else if err != event.ErrEventNotFound { // todo: can be dropped when older gift cards (prior to tracking) have been resolved prior
			return err
		}
	}

	return nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

type EstablishRelationshipIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
}

func NewEstablishRelationshipIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard) CreateIntentHandler {
	return &EstablishRelationshipIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
	}
}

func (h *EstablishRelationshipIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetEstablishRelationship()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	intentRecord.IntentType = intent.EstablishRelationship
	intentRecord.EstablishRelationshipMetadata = &intent.EstablishRelationshipMetadata{
		RelationshipTo: typedProtoMetadata.Relationship.GetDomain().Value,
	}

	return nil
}

func (h *EstablishRelationshipIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	openAction := actions[0].GetOpenAccount()
	if openAction == nil {
		// Something is off, send it to validation
		return false, nil
	}
	authority, err := common.NewAccountFromProto(openAction.Authority)
	if err != nil {
		return false, err
	}

	// Ensure the authority is the same, otherwise something is off and this
	// intent should be sent off to validation.
	existingAccountInfoRecord, err := h.data.GetRelationshipAccountInfoByOwnerAddress(ctx, intentRecord.InitiatorOwnerAccount, intentRecord.EstablishRelationshipMetadata.RelationshipTo)
	if err == nil && existingAccountInfoRecord.AuthorityAccount == authority.PublicKey().ToBase58() {
		return true, nil
	} else if err != account.ErrAccountInfoNotFound {
		return false, err
	}
	return false, nil
}

func (h *EstablishRelationshipIntentHandler) GetAdditionalAccountsToLock(ctx context.Context, intentRecord *intent.Record) (*lockableAccounts, error) {
	return &lockableAccounts{}, nil
}

func (h *EstablishRelationshipIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action, deviceToken *string) error {
	typedMetadata := metadata.GetEstablishRelationship()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}
	reslationshipTo := typedMetadata.Relationship.GetDomain().Value

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	//
	// Part 1: Intent ID validation
	//

	err = validateIntentIdIsNotRequest(ctx, h.data, intentRecord.IntentId)
	if err != nil {
		return err
	}

	//
	// Part 2: Antispam checks against the phone number
	//

	if !h.conf.disableAntispamChecks.Get(ctx) {
		allow, err := h.antispamGuard.AllowEstablishNewRelationship(ctx, initiatiorOwnerAccount, reslationshipTo)
		if err != nil {
			return err
		} else if !allow {
			return ErrTooManyNewRelationships
		}
	}

	//
	// Part 3: User accounts must be opened
	//

	_, err = h.data.GetLatestIntentByInitiatorAndType(ctx, intent.OpenAccounts, initiatiorOwnerAccount.PublicKey().ToBase58())
	if err == intent.ErrIntentNotFound {
		return newIntentDeniedError("open accounts intent not submitted")
	} else if err != nil {
		return err
	}

	//
	// Part 4: Relationship identifier validation
	//

	asciiBaseDomain, err := thirdparty.GetAsciiBaseDomain(reslationshipTo)
	if err != nil || asciiBaseDomain != intentRecord.EstablishRelationshipMetadata.RelationshipTo {
		return newIntentValidationError("domain is not an ascii base domain")
	}

	//
	// Part 5: Validate the owner hasn't already established a relationship
	//

	existingAccountInfoRecord, err := h.data.GetRelationshipAccountInfoByOwnerAddress(ctx, initiatiorOwnerAccount.PublicKey().ToBase58(), reslationshipTo)
	if err == nil {
		return newStaleStateErrorf("existing relationship account exists with authority %s", existingAccountInfoRecord.AuthorityAccount)
	} else if err != account.ErrAccountInfoNotFound {
		return err
	}

	//
	// Part 6: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 7: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 8: Validate the individual actions
	//

	return h.validateActions(initiatiorOwnerAccount, actions)
}

func (h *EstablishRelationshipIntentHandler) validateActions(initiatiorOwnerAccount *common.Account, actions []*transactionpb.Action) error {
	if len(actions) != 1 {
		return newIntentValidationError("expected 1 action")
	}

	openAction := actions[0]
	if openAction.GetOpenAccount() == nil {
		return newActionValidationError(openAction, "expected an open account action")
	}

	if openAction.GetOpenAccount().AccountType != commonpb.AccountType_RELATIONSHIP {
		return newActionValidationErrorf(openAction, "account type must be %s", commonpb.AccountType_RELATIONSHIP)
	}

	if openAction.GetOpenAccount().Index != 0 {
		return newActionValidationError(openAction, "index must be 0")
	}

	if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatiorOwnerAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "owner must be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
	}

	if bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
		return newActionValidationErrorf(openAction, "authority cannot be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
	}

	return nil
}

func (h *EstablishRelationshipIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func (h *EstablishRelationshipIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func validateAllUserAccountsManagedByCode(ctx context.Context, initiatorAccounts []*common.AccountRecords) error {
	// Try to unlock *ANY* latest account, and you're done
	for _, accountRecords := range initiatorAccounts {
		if !accountRecords.IsManagedByCode(ctx) {
			return ErrNotManagedByCode
		}
	}

	return nil
}

func validateBucketAccountSimulation(accountType commonpb.AccountType, simulation TokenAccountSimulation) error {
	publicTransfers := simulation.GetPublicTransfers()
	if len(publicTransfers) > 0 {
		return newActionValidationError(publicTransfers[0].Action, "bucket account cannot send/receive kin publicly")
	}

	withdraws := simulation.GetWithdraws()
	if len(withdraws) > 0 {
		return newActionValidationError(withdraws[0].Action, "bucket account cannot send/receive kin using withdraw")
	}

	bucketSize, ok := bucketSizeByAccountType[accountType]
	if !ok {
		return errors.New("bucket size is not defined")
	}

	for _, transfer := range simulation.Transfers {
		if transfer.DeltaQuarks%int64(bucketSize) != 0 {
			return newActionValidationErrorf(transfer.Action, "quark amount must be a multiple of %d kin", kin.FromQuarks(bucketSize))
		}

		absDeltaQuarks := transfer.DeltaQuarks
		if absDeltaQuarks < 0 {
			absDeltaQuarks = -absDeltaQuarks
		}

		_, anonymized := allowedBucketQuarkAmounts[uint64(absDeltaQuarks)]
		if !anonymized {
			return newActionValidationError(transfer.Action, "quark amount must be anonymized")
		}
	}

	return nil
}

func validateMoneyMovementActionCount(actions []*transactionpb.Action) error {
	var numMoneyMovementActions int

	for _, action := range actions {
		switch action.Type.(type) {
		case *transactionpb.Action_NoPrivacyWithdraw,
			*transactionpb.Action_NoPrivacyTransfer,
			*transactionpb.Action_TemporaryPrivacyTransfer,
			*transactionpb.Action_TemporaryPrivacyExchange,
			*transactionpb.Action_FeePayment:

			numMoneyMovementActions++
		}
	}

	// todo: configurable
	if numMoneyMovementActions > 50 {
		return newIntentDeniedError("too many transfer/exchange/withdraw actions")
	}
	return nil
}

// Provides generic and lightweight validation of which accounts owned by a Code
// user can be used in certain actions. This is by no means a comprehensive check.
// Other account types (eg. gift cards, external wallets, etc) and intent-specific
// complex nuances should be handled elsewhere.
func validateMoneyMovementActionUserAccounts(
	intentType intent.Type,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	actions []*transactionpb.Action,
) error {
	for _, action := range actions {
		var authority, source *common.Account
		var err error

		switch typedAction := action.Type.(type) {
		case *transactionpb.Action_NoPrivacyTransfer:
			// No privacy transfers are always come from a deposit account

			authority, err = common.NewAccountFromProto(typedAction.NoPrivacyTransfer.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.NoPrivacyTransfer.Source)
			if err != nil {
				return err
			}

			sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
			if !ok || (sourceAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY && sourceAccountInfo.General.AccountType != commonpb.AccountType_RELATIONSHIP) {
				return newActionValidationError(action, "source account must be a deposit account")
			}
		case *transactionpb.Action_NoPrivacyWithdraw:
			// No privacy withdraws are used in two ways depending on the intent:
			//  1. As the sender of funds from the latest temp outgoing account in a private send
			//  2. As a receiver of funds to the latest temp incoming account in a public receive

			authority, err = common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Source)
			if err != nil {
				return err
			}

			destination, err := common.NewAccountFromProto(typedAction.NoPrivacyWithdraw.Destination)
			if err != nil {
				return err
			}

			switch intentType {
			case intent.SendPrivatePayment:
				sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
				if !ok || sourceAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_OUTGOING {
					return newActionValidationError(action, "source account must be the latest temporary outgoing account")
				}
			case intent.ReceivePaymentsPublicly:
				destinationAccountInfo, ok := initiatorAccountsByVault[destination.PublicKey().ToBase58()]
				if !ok || destinationAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
					return newActionValidationError(action, "source account must be the latest temporary incoming account")
				}
			}
		case *transactionpb.Action_TemporaryPrivacyTransfer:
			// Temporary privacy transfers are always between Code user accounts
			// where one of the source or destination account is a bucket.

			authority, err = common.NewAccountFromProto(typedAction.TemporaryPrivacyTransfer.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.TemporaryPrivacyTransfer.Source)
			if err != nil {
				return err
			}

			destination, err := common.NewAccountFromProto(typedAction.TemporaryPrivacyTransfer.Destination)
			if err != nil {
				return err
			}

			sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
			if !ok {
				return newActionValidationError(action, "source account is not a latest owned account")
			}

			destinationAccountInfo, ok := initiatorAccountsByVault[destination.PublicKey().ToBase58()]
			if !ok {
				return newActionValidationError(action, "destination account is not a latest owned account")
			}

			if sourceAccountInfo.General.IsBucket() && destinationAccountInfo.General.IsBucket() {
				return newActionValidationError(action, "expected an exchange action")
			}

			if !sourceAccountInfo.General.IsBucket() && !destinationAccountInfo.General.IsBucket() {
				return newActionValidationError(action, "bucket account must be involved in a private transfer")
			}

			if !sourceAccountInfo.General.IsBucket() &&
				sourceAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_INCOMING &&
				sourceAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY &&
				sourceAccountInfo.General.AccountType != commonpb.AccountType_RELATIONSHIP {
				return newActionValidationError(action, "source account must be a deposit or latest temporary incoming account when not a bucket account")
			}

			if !destinationAccountInfo.General.IsBucket() && destinationAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_OUTGOING {
				return newActionValidationError(action, "destination account must be the latest temporary outgoing account when not a bucket account")
			}
		case *transactionpb.Action_TemporaryPrivacyExchange:
			// Temporary privacy exchanges are always between bucket accounts

			authority, err = common.NewAccountFromProto(typedAction.TemporaryPrivacyExchange.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.TemporaryPrivacyExchange.Source)
			if err != nil {
				return err
			}

			destination, err := common.NewAccountFromProto(typedAction.TemporaryPrivacyExchange.Destination)
			if err != nil {
				return err
			}

			sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
			if !ok || !sourceAccountInfo.General.IsBucket() {
				return newActionValidationError(action, "source account must be an owned bucket account")
			}

			destinationAccountInfo, ok := initiatorAccountsByVault[destination.PublicKey().ToBase58()]
			if !ok || !destinationAccountInfo.General.IsBucket() {
				return newActionValidationError(action, "destination account must be an owned bucket account")
			}
		case *transactionpb.Action_FeePayment:
			// Fee payments always come from the latest temporary outgoing account

			authority, err = common.NewAccountFromProto(typedAction.FeePayment.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.FeePayment.Source)
			if err != nil {
				return err
			}

			sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
			if !ok || sourceAccountInfo.General.AccountType != commonpb.AccountType_TEMPORARY_OUTGOING {
				return newActionValidationError(action, "source account must be the latest temporary outgoing account")
			}
		default:
			continue
		}

		expectedTimelockVault, err := getExpectedTimelockVaultFromProtoAccount(authority.ToProto())
		if err != nil {
			return err
		} else if !bytes.Equal(expectedTimelockVault.PublicKey().ToBytes(), source.PublicKey().ToBytes()) {
			return newActionValidationErrorf(action, "authority is invalid")
		}
	}

	return nil
}

func validateNextTemporaryAccountOpened(
	accountType commonpb.AccountType,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	actions []*transactionpb.Action,
) error {
	if accountType != commonpb.AccountType_TEMPORARY_INCOMING && accountType != commonpb.AccountType_TEMPORARY_OUTGOING {
		return errors.New("unexpected account type")
	}

	prevTempAccountRecords, ok := initiatorAccountsByType[accountType]
	if !ok {
		return errors.New("previous temp account record missing")
	}

	// Find the open and close actions

	var openAction *transactionpb.Action
	for _, action := range actions {
		switch typed := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			if typed.OpenAccount.AccountType == accountType {
				if openAction != nil {
					return newIntentValidationErrorf("multiple open actions for %s account type", accountType)
				}

				openAction = action
			}
		}
	}

	if openAction == nil {
		return newIntentValidationErrorf("open account action for %s account type missing", accountType)
	}

	if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatorOwnerAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "owner must be %s", initiatorOwnerAccount.PublicKey().ToBase58())
	}

	if bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
		return newActionValidationErrorf(openAction, "authority cannot be %s", initiatorOwnerAccount.PublicKey().ToBase58())
	}

	expectedIndex := prevTempAccountRecords[0].General.Index + 1
	if openAction.GetOpenAccount().Index != expectedIndex {
		return newActionValidationErrorf(openAction, "next derivation expected to be %d", expectedIndex)
	}

	expectedVaultAccount, err := getExpectedTimelockVaultFromProtoAccount(openAction.GetOpenAccount().Authority)
	if err != nil {
		return err
	}

	if !bytes.Equal(openAction.GetOpenAccount().Token.Value, expectedVaultAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "token must be %s", expectedVaultAccount.PublicKey().ToBase58())
	}

	return nil
}

// Assumes only one gift card account is opened per intent
func validateGiftCardAccountOpened(
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	expectedGiftCardVault *common.Account,
	actions []*transactionpb.Action,
) error {
	var openAction *transactionpb.Action
	for _, action := range actions {
		switch typed := action.Type.(type) {
		case *transactionpb.Action_OpenAccount:
			if typed.OpenAccount.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
				if openAction != nil {
					return newIntentValidationErrorf("multiple open actions for %s account type", commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
				}

				openAction = action
			}
		}
	}

	if openAction == nil {
		return newIntentValidationErrorf("open account action for %s account type missing", commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
	}

	if bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatorOwnerAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "owner cannot be %s", initiatorOwnerAccount.PublicKey().ToBase58())
	}

	if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
		return newActionValidationErrorf(openAction, "authority must be %s", openAction.GetOpenAccount().Owner.Value)
	}

	if openAction.GetOpenAccount().Index != 0 {
		return newActionValidationError(openAction, "index must be 0")
	}

	derivedVaultAccount, err := getExpectedTimelockVaultFromProtoAccount(openAction.GetOpenAccount().Authority)
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedGiftCardVault.PublicKey().ToBytes(), derivedVaultAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "token must be %s", expectedGiftCardVault.PublicKey().ToBase58())
	}

	if !bytes.Equal(openAction.GetOpenAccount().Token.Value, derivedVaultAccount.PublicKey().ToBytes()) {
		return newActionValidationErrorf(openAction, "token must be %s", derivedVaultAccount.PublicKey().ToBase58())
	}

	return nil
}

func validateNoUpgradeActions(actions []*transactionpb.Action) error {
	for _, action := range actions {
		switch action.Type.(type) {
		case *transactionpb.Action_PermanentPrivacyUpgrade:
			return newActionValidationError(action, "update action not allowed")
		}
	}

	return nil
}

func validateExternalKinTokenAccountWithinIntent(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) error {
	/*
		isValid, message, err := common.ValidateExternalKinTokenAccount(ctx, data, tokenAccount)
		if err != nil {
			return err
		} else if !isValid {
			return newIntentValidationError(message)
		}
		return nil
	*/
	return newIntentDeniedError("external transfers are not yet supported")
}

func validateExchangeDataWithinIntent(ctx context.Context, data code_data.Provider, intentId string, proto *transactionpb.ExchangeData) error {
	// If there's a payment request record, then validate exchange data client
	// provided matches exactly. The payment request record should already have
	// validated exchange data before it was created.
	requestRecord, err := data.GetRequest(ctx, intentId)
	if err == nil {
		if !requestRecord.RequiresPayment() {
			return newIntentValidationError("request doesn't require payment")
		}

		if proto.Currency != string(*requestRecord.ExchangeCurrency) {
			return newIntentValidationErrorf("payment has a request for %s currency", *requestRecord.ExchangeCurrency)
		}

		absNativeAmountDiff := math.Abs(proto.NativeAmount - *requestRecord.NativeAmount)
		if absNativeAmountDiff > 0.0001 {
			return newIntentValidationErrorf("payment has a request for %.2f native amount", *requestRecord.NativeAmount)
		}

		// No need to validate exchange details in the payment request. Only Kin has
		// exact exchange data requirements, which has already been validated at time
		// of payment intent creation. We do leave the ability open to reserve an exchange
		// rate, but no use cases warrant that atm.

	} else if err != paymentrequest.ErrPaymentRequestNotFound {
		return err
	}

	// Otherwise, validate exchange data fully using the common method
	isValid, message, err := exchange_rate_util.ValidateClientExchangeData(ctx, data, proto)
	if err != nil {
		return err
	} else if !isValid {
		if strings.Contains(message, "stale") {
			return newStaleStateError(message)
		}
		return newIntentValidationError(message)
	}
	return nil
}

// Generically validates fee payments as much as possible, but won't cover any
// intent-specific nuances (eg. where the fee payment comes from)
//
// This assumes source and destination accounts interacting with fees and the
// remaining amount don't have minimum bucket size requirements. Intent validation
// logic is responsible for these checks and guarantees.
func validateFeePayments(
	ctx context.Context,
	data code_data.Provider,
	intentRecord *intent.Record,
	simResult *LocalSimulationResult,
) error {
	var requiresFee bool
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		requiresFee = intentRecord.SendPrivatePaymentMetadata.IsMicroPayment
	}

	if !requiresFee && simResult.HasAnyFeePayments() {
		return newIntentValidationError("intent doesn't require a fee payment")
	}

	if requiresFee && !simResult.HasAnyFeePayments() {
		return newIntentValidationError("intent requires a fee payment")
	}

	if !requiresFee {
		return nil
	}

	requestRecord, err := data.GetRequest(ctx, intentRecord.IntentId)
	if err != nil {
		return err
	}

	if !requestRecord.RequiresPayment() {
		return newIntentValidationError("request doesn't require payment")
	}

	additionalRequestedFees := requestRecord.Fees

	feePayments := simResult.GetFeePayments()
	if len(feePayments) != len(additionalRequestedFees)+1 {
		return newIntentValidationErrorf("expected %d fee payment action", len(additionalRequestedFees)+1)
	}

	codeFeePayment := feePayments[0]

	if codeFeePayment.Action.GetFeePayment().Type != transactionpb.FeePaymentAction_CODE {
		return newActionValidationError(codeFeePayment.Action, "fee payment type must be CODE")
	}

	if codeFeePayment.Action.GetFeePayment().Destination != nil {
		return newActionValidationError(codeFeePayment.Action, "code fee payment destination is configured by server")
	}

	feeAmount := codeFeePayment.DeltaQuarks
	if feeAmount >= 0 {
		return newActionValidationError(codeFeePayment.Action, "fee payment amount is negative")
	}
	feeAmount = -feeAmount // Because it's coming out of a user account in this simulation

	var foundUsdExchangeRecord bool
	usdExchangeRecords, err := exchange_rate_util.GetPotentialClientExchangeRates(ctx, data, currency_lib.USD)
	if err != nil {
		return err
	}
	for _, exchangeRecord := range usdExchangeRecords {
		usdValue := exchangeRecord.Rate * float64(feeAmount) / float64(kin.QuarksPerKin)

		// Allow for some small margin of error
		//
		// todo: Hardcoded as a penny USD, but might want a dynamic amount if we
		//       have use cases with different fee amounts.
		if usdValue > 0.0099 && usdValue < 0.0101 {
			foundUsdExchangeRecord = true
			break
		}
	}

	if !foundUsdExchangeRecord {
		return newActionValidationError(codeFeePayment.Action, "code fee payment amount must be $0.01 USD")
	}

	for i, additionalFee := range feePayments[1:] {
		if additionalFee.Action.GetFeePayment().Type != transactionpb.FeePaymentAction_THIRD_PARTY {
			return newActionValidationError(additionalFee.Action, "fee payment type must be THIRD_PARTY")
		}

		destination := additionalFee.Action.GetFeePayment().Destination
		if destination == nil {
			return newActionValidationError(additionalFee.Action, "fee payment destination is required")
		}

		// The destination should already be validated as a valid payment destination
		if base58.Encode(destination.Value) != additionalRequestedFees[i].DestinationTokenAccount {
			return newActionValidationErrorf(additionalFee.Action, "fee payment destination must be %s", additionalRequestedFees[i].DestinationTokenAccount)
		}

		feeAmount := additionalFee.DeltaQuarks
		if feeAmount >= 0 {
			return newActionValidationError(additionalFee.Action, "fee payment amount is negative")
		}
		feeAmount = -feeAmount // Because it's coming out of a user account in this simulation

		requestedAmount := (uint64(additionalRequestedFees[i].BasisPoints) * intentRecord.SendPrivatePaymentMetadata.Quantity) / 10000
		if feeAmount != int64(requestedAmount) {
			return newActionValidationErrorf(additionalFee.Action, "fee payment amount must be for %d bps of total amount", additionalRequestedFees[i].BasisPoints)
		}
	}

	return nil
}

func validateMinimalTempIncomingAccountUsage(ctx context.Context, data code_data.Provider, accountInfo *account.Record) error {
	if accountInfo.AccountType != commonpb.AccountType_TEMPORARY_INCOMING {
		return errors.New("expected a temporary incoming account")
	}

	actionRecords, err := data.GetAllActionsByAddress(ctx, accountInfo.TokenAccount)
	if err != nil && err != action.ErrActionNotFound {
		return err
	}

	var paymentCount int
	for _, actionRecord := range actionRecords {
		// Revoked actions don't count
		if actionRecord.State == action.StateRevoked {
			continue
		}

		// Temp incoming accounts are always paid via no privacy withdraws
		if actionRecord.ActionType != action.NoPrivacyWithdraw {
			continue
		}

		paymentCount += 1
	}

	// Should be coordinated with MustRotate flag in GetTokenAccountInfos
	//
	// todo: configurable
	if paymentCount >= 2 {
		// Important Note: Do not leak anything. Just say it isn't the latest.
		return newStaleStateError("destination is not the latest temporary incoming account")
	}
	return nil
}

func validateClaimedGiftCard(ctx context.Context, data code_data.Provider, giftCardVaultAccount *common.Account, claimedAmount uint64) error {
	//
	// Part 1: Is the account a gift card?
	//

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound || accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return newIntentValidationError("source is not a remote send gift card")
	}

	//
	// Part 2: Is there already an action to claim the gift card balance?
	//

	_, err = data.GetGiftCardClaimedAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == nil {
		return newStaleStateError("gift card balance has already been claimed")
	} else if err == action.ErrActionNotFound {
		// No action to claim it, so we can proceed
	} else if err != nil {
		return err
	}

	//
	// Part 3: Is the action to auto-return the balance back to the issuer in a scheduling or post-scheduling state?
	//

	autoReturnActionRecord, err := data.GetGiftCardAutoReturnAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err != nil && err != action.ErrActionNotFound {
		return err
	}

	if err == nil && autoReturnActionRecord.State != action.StateUnknown {
		return newStaleStateError("gift card is expired")
	}

	//
	// Part 4: Is the gift card managed by Code?
	//

	timelockRecord, err := data.GetTimelockByVault(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	isManagedByCode := common.IsManagedByCode(ctx, timelockRecord)
	if err != nil {
		return err
	} else if !isManagedByCode {
		if timelockRecord.IsClosed() {
			// Better error messaging, since we know we'll never reopen the account
			// and the balance is guaranteed to be claimed (not necessarily through
			// Code server though).
			return newStaleStateError("gift card balance has already been claimed")
		}
		return ErrNotManagedByCode
	}

	//
	// Part 5: Is the full amount being claimed?
	//

	// We don't track external deposits to gift cards or any further Code transfers
	// to it in SubmitIntent, so this check is sufficient for now.
	giftCardBalance, err := balance.CalculateFromCache(ctx, data, giftCardVaultAccount)
	if err != nil {
		return err
	} else if giftCardBalance == 0 {
		// Shouldn't be hit with checks from part 3, but left for completeness
		return newStaleStateError("gift card balance has already been claimed")
	} else if giftCardBalance != claimedAmount {
		return newIntentValidationErrorf("must receive entire gift card balance of %d quarks", giftCardBalance)
	}

	//
	// Part 6: Are we within the threshold for auto-return back to the issuer?
	//

	// todo: I think we use the same trick of doing deadline - x minutes to avoid race
	//       conditions without distributed locks.
	if time.Since(accountInfoRecord.CreatedAt) > 24*time.Hour-15*time.Minute {
		return newStaleStateError("gift card is expired")
	}

	return nil
}

func validateIntentIdIsNotRequest(ctx context.Context, data code_data.Provider, intentId string) error {
	_, err := data.GetRequest(ctx, intentId)
	if err == nil {
		return newIntentDeniedError("intent id is reserved for a request")
	} else if err != paymentrequest.ErrPaymentRequestNotFound {
		return err
	}
	return nil
}

func validateTipDestination(ctx context.Context, data code_data.Provider, tippedUser *transactionpb.TippedUser, actualDestination *common.Account) error {
	var expectedDestination *common.Account
	switch tippedUser.Platform {
	case transactionpb.TippedUser_TWITTER:
		record, err := data.GetTwitterUserByUsername(ctx, tippedUser.Username)
		if err == twitter.ErrUserNotFound {
			return newIntentValidationError("twitter user is not registered with code")
		} else if err != nil {
			return err
		}

		expectedDestination, err = common.NewAccountFromPublicKeyString(record.TipAddress)
		if err != nil {
			return err
		}
	default:
		return newIntentValidationErrorf("tip platform %s is not supported", tippedUser.Platform.String())
	}

	if !bytes.Equal(expectedDestination.PublicKey().ToBytes(), actualDestination.PublicKey().ToBytes()) {
		return newIntentValidationErrorf("tip destination must be %s", expectedDestination.PublicKey().ToBase58())
	}

	return nil
}

func getExpectedTimelockVaultFromProtoAccount(authorityProto *commonpb.SolanaAccountId) (*common.Account, error) {
	authorityAccount, err := common.NewAccountFromProto(authorityProto)
	if err != nil {
		return nil, err
	}

	timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
	if err != nil {
		return nil, err
	}
	return timelockAccounts.Vault, nil
}
