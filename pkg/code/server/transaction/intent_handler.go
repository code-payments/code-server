package transaction_v2

import (
	"bytes"
	"context"
	"math"
	"strings"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana"
)

var accountTypesToOpen = []commonpb.AccountType{
	commonpb.AccountType_PRIMARY,
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
	AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) error

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
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
}

func NewOpenAccountsIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard) CreateIntentHandler {
	return &OpenAccountsIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
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

func (h *OpenAccountsIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
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
		allow, err := h.antispamGuard.AllowOpenAccounts(ctx, initiatiorOwnerAccount)
		if err != nil {
			return err
		} else if !allow {
			return newIntentDeniedError("antispam guard denied account creation")
		}
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

	err = h.validateActions(ctx, initiatiorOwnerAccount, actions)
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

func (h *OpenAccountsIntentHandler) validateActions(ctx context.Context, initiatiorOwnerAccount *common.Account, actions []*transactionpb.Action) error {
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

		if err := validateTimelockUnlockStateDoesntExist(ctx, h.data, openAction.GetOpenAccount()); err != nil {
			return err
		}
	}

	return nil
}

func (h *OpenAccountsIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func (h *OpenAccountsIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

type SendPublicPaymentIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
}

func NewSendPublicPaymentIntentHandler(
	conf *conf,
	data code_data.Provider,
	antispamGuard *antispam.Guard,
) CreateIntentHandler {
	return &SendPublicPaymentIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
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
		UsdMarketValue:   usdExchangeRecord.Rate * float64(exchangeData.Quarks) / float64(common.CoreMintQuarksPerUnit),

		IsWithdrawal: typedProtoMetadata.IsWithdrawal,
	}

	if destinationAccountInfo != nil {
		intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount = destinationAccountInfo.OwnerAccount
	}

	if intentRecord.SendPublicPaymentMetadata.IsRemoteSend && intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
		return newIntentValidationError("remote send cannot be a withdraw")
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

func (h *SendPublicPaymentIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
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

		allow, err := h.antispamGuard.AllowSendPayment(ctx, initiatiorOwnerAccount, destination, true)
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
		typedMetadata,
		actions,
		simResult,
	)
}

// todo: For remote send, we still need to fully validate the auto-return action
func (h *SendPublicPaymentIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	metadata *transactionpb.SendPublicPaymentMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	if !metadata.IsRemoteSend && len(actions) != 1 {
		return newIntentValidationError("expected 1 action")
	}
	if metadata.IsRemoteSend && len(actions) != 3 {
		return newIntentValidationError("expected 3 actions")
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
		// Remote sends must be to a brand new gift card account
		if metadata.IsRemoteSend {
			return newIntentValidationError("destination must be a brand new gift card account")
		}

		// Code->Code public withdraws must be done against other deposit accounts
		if metadata.IsWithdrawal && destinationAccountInfo.AccountType != commonpb.AccountType_PRIMARY {
			return newIntentValidationError("destination account must be a deposit account")
		}

		// And the destination cannot be the source of funds, since that results in a no-op
		if source.PublicKey().ToBase58() == destinationAccountInfo.TokenAccount {
			return newIntentValidationError("payment is a no-op")
		}
	case account.ErrAccountInfoNotFound:
		// Check whether the destination account is a core mint token account that's
		// been created on the blockchain. Exception is made when we're doing a remote
		// send, since we expect the gift card account to no yet exist.
		if !metadata.IsRemoteSend && !h.conf.disableBlockchainChecks.Get(ctx) {
			err = validateExternalTokenAccountWithinIntent(ctx, h.data, destination)
			if err != nil {
				return err
			}
		}
	default:
		return err
	}

	sourceAccountRecords, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
	if !ok || sourceAccountRecords.General.AccountType != commonpb.AccountType_PRIMARY {
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
	} else if destinationSimulation.GetDeltaQuarks(false) != int64(metadata.ExchangeData.Quarks) {
		return newActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", metadata.ExchangeData.Quarks)
	}

	//
	// Part 2.2: Check that the user's deposit account was used as the source of funds
	//           as specified in the metadata
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment from source account %s", source.PublicKey().ToBase58())
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(metadata.ExchangeData.Quarks) {
		return newActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must send %d quarks from source account", metadata.ExchangeData.Quarks)
	}

	// Part 3: Generic validation of actions that move money

	err = validateMoneyMovementActionUserAccounts(intent.SendPublicPayment, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	// Part 4: Validate open and closed accounts

	if metadata.IsRemoteSend {
		if len(simResult.GetOpenedAccounts()) != 1 {
			return newIntentValidationError("expected 1 account opened")
		}

		err = validateGiftCardAccountOpened(
			ctx,
			h.data,
			initiatorOwnerAccount,
			destination,
			actions,
		)
		if err != nil {
			return err
		}

		closedAccounts := simResult.GetClosedAccounts()
		if len(FilterAutoReturnedAccounts(closedAccounts)) != 0 {
			return newIntentValidationError("expected no closed accounts outside of auto-returns")
		}
		if len(closedAccounts) != 1 {
			return newIntentValidationError("expected exactly 1 auto-returned account")
		}

		autoReturns := destinationSimulation.GetAutoReturns()
		if len(autoReturns) != 1 {
			return newIntentValidationError("expected auto-return for the remote send gift card")
		} else if autoReturns[0].IsPrivate || !autoReturns[0].IsWithdraw {
			return newActionValidationError(destinationSimulation.Transfers[0].Action, "auto-return must be a public withdraw")
		} else if autoReturns[0].DeltaQuarks != -int64(metadata.ExchangeData.Quarks) {
			return newActionValidationErrorf(autoReturns[0].Action, "must auto-return %d quarks from remote send gift card", metadata.ExchangeData.Quarks)
		}

		autoReturns = sourceSimulation.GetAutoReturns()
		if len(autoReturns) != 1 {
			return newIntentValidationError("gift card auto-return balance must go to the source account")
		}
	} else {
		if len(simResult.GetOpenedAccounts()) > 0 {
			return newIntentValidationError("cannot open any account")
		}

		if len(simResult.GetClosedAccounts()) > 0 {
			return newIntentValidationError("cannot close any account")
		}
	}

	return nil
}

func (h *SendPublicPaymentIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func (h *SendPublicPaymentIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

// Generally needs a rewrite to send funds to the primary account
type ReceivePaymentsPubliclyIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard

	cachedGiftCardIssuedIntentRecord *intent.Record
}

func NewReceivePaymentsPubliclyIntentHandler(conf *conf, data code_data.Provider, antispamGuard *antispam.Guard) CreateIntentHandler {
	return &ReceivePaymentsPubliclyIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
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
		Source:   giftCardVault.PublicKey().ToBase58(),
		Quantity: typedProtoMetadata.Quarks,

		IsRemoteSend:            typedProtoMetadata.IsRemoteSend,
		IsReturned:              false,
		IsIssuerVoidingGiftCard: typedProtoMetadata.IsIssuerVoidingGiftCard,

		OriginalExchangeCurrency: giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency,
		OriginalExchangeRate:     giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeRate,
		OriginalNativeAmount:     giftCardIssuedIntentRecord.SendPublicPaymentMetadata.NativeAmount,

		UsdMarketValue: usdExchangeRecord.Rate * float64(typedProtoMetadata.Quarks) / float64(common.CoreMintQuarksPerUnit),
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

func (h *ReceivePaymentsPubliclyIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
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
		initiatorAccountsByType,
		initiatorAccountsByVault,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *ReceivePaymentsPubliclyIntentHandler) validateActions(
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

	// The destination account must be the primary
	destinationAccountInfo := initiatorAccountsByType[commonpb.AccountType_PRIMARY][0].General
	destinationSimulation, ok := simResult.SimulationsByAccount[destinationAccountInfo.TokenAccount]
	if !ok {
		return newActionValidationError(actions[0], "must send payment to primary account")
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
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(metadata.Quarks) {
		return newActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must receive %d quarks from source account", metadata.Quarks)
	} else if sourceSimulation.Transfers[0].IsPrivate || !sourceSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(sourceSimulation.Transfers[0].Action, "transfer must be a public withdraw")
	}

	//
	// Part 2.2: Check destination account is paid exact quark amount from source account in a public withdraw
	//

	if destinationSimulation.GetDeltaQuarks(false) != int64(metadata.Quarks) {
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
	return nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
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
			if !ok || sourceAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY {
				return newActionValidationError(action, "source account must be a deposit account")
			}
		case *transactionpb.Action_NoPrivacyWithdraw:
			// No privacy withdraws are used in two ways depending on the intent:
			//  1. As an auto-return action back to the payer's primary account in a public payment intent for remote send
			//  2. As a receiver of funds to the primary account in a public receive

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
			case intent.SendPublicPayment, intent.ReceivePaymentsPublicly:
				destinationAccountInfo, ok := initiatorAccountsByVault[destination.PublicKey().ToBase58()]
				if !ok || destinationAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY {
					return newActionValidationError(action, "source account must be the primary account")
				}
			}
		case *transactionpb.Action_FeePayment:
			// Fee payments always come from the primary account

			authority, err = common.NewAccountFromProto(typedAction.FeePayment.Authority)
			if err != nil {
				return err
			}

			source, err = common.NewAccountFromProto(typedAction.FeePayment.Source)
			if err != nil {
				return err
			}

			sourceAccountInfo, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
			if !ok || sourceAccountInfo.General.AccountType != commonpb.AccountType_PRIMARY {
				return newActionValidationError(action, "source account must be the primary account")
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

// Assumes only one gift card account is opened per intent
func validateGiftCardAccountOpened(
	ctx context.Context,
	data code_data.Provider,
	initiatorOwnerAccount *common.Account,
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

	if err := validateTimelockUnlockStateDoesntExist(ctx, data, openAction.GetOpenAccount()); err != nil {
		return err
	}

	return nil
}

func validateExternalTokenAccountWithinIntent(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) error {
	isValid, message, err := common.ValidateExternalTokenAccount(ctx, data, tokenAccount)
	if err != nil {
		return err
	} else if !isValid {
		return newIntentValidationError(message)
	}
	return nil
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
	var totalQuarksSent uint64
	switch intentRecord.IntentType {
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
		usdValue := exchangeRecord.Rate * float64(feeAmount) / float64(common.CoreMintQuarksPerUnit)

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

		requestedAmount := (uint64(additionalRequestedFees[i].BasisPoints) * totalQuarksSent) / 10000
		if feeAmount != int64(requestedAmount) {
			return newActionValidationErrorf(additionalFee.Action, "fee payment amount must be for %d bps of total amount", additionalRequestedFees[i].BasisPoints)
		}
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
	if !isManagedByCode {
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

func validateTimelockUnlockStateDoesntExist(ctx context.Context, data code_data.Provider, openAction *transactionpb.OpenAccountAction) error {
	authorityAccount, err := common.NewAccountFromProto(openAction.Authority)
	if err != nil {
		return err
	}

	timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return err
	}

	_, err = data.GetBlockchainAccountInfo(ctx, timelockAccounts.Unlock.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		return newIntentDeniedError("an account being opened has already initiated an unlock")
	case solana.ErrNoAccountInfo:
		return nil
	default:
		return err
	}
}

func getExpectedTimelockVaultFromProtoAccount(authorityProto *commonpb.SolanaAccountId) (*common.Account, error) {
	authorityAccount, err := common.NewAccountFromProto(authorityProto)
	if err != nil {
		return nil, err
	}

	timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}
	return timelockAccounts.Vault, nil
}
