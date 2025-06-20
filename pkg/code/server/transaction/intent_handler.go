package transaction_v2

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/aml"
	"github.com/code-payments/code-server/pkg/code/antispam"
	async_account "github.com/code-payments/code-server/pkg/code/async/account"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana"
)

var accountTypesToOpen = []commonpb.AccountType{
	commonpb.AccountType_PRIMARY,
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

	// GetAccountsWithBalancesToLock gets a set of accounts with balances that need
	// to be locked.
	GetAccountsWithBalancesToLock(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*common.Account, error)

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

func (h *OpenAccountsIntentHandler) GetAccountsWithBalancesToLock(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*common.Account, error) {
	return nil, nil
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
	// Part 1: Antispam checks against the owner
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
	// Part 2: Validate the owner hasn't already created an OpenAccounts intent
	//

	_, err = h.data.GetLatestIntentByInitiatorAndType(ctx, intent.OpenAccounts, initiatiorOwnerAccount.PublicKey().ToBase58())
	if err == nil {
		return newStaleStateError("already submitted intent to open accounts")
	} else if err != intent.ErrIntentNotFound {
		return err
	}

	//
	// Part 3: Validate the individual actions
	//

	err = h.validateActions(ctx, initiatiorOwnerAccount, actions)
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

	return validateFeePayments(ctx, h.data, h.conf, intentRecord, simResult)
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
	amlGuard      *aml.Guard
}

func NewSendPublicPaymentIntentHandler(
	conf *conf,
	data code_data.Provider,
	antispamGuard *antispam.Guard,
	amlGuard *aml.Guard,
) CreateIntentHandler {
	return &SendPublicPaymentIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,
	}
}

func (h *SendPublicPaymentIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetSendPublicPayment()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	exchangeData := typedProtoMetadata.ExchangeData

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, currency_util.GetLatestExchangeRateTime())
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
		IsRemoteSend: typedProtoMetadata.IsRemoteSend,
	}

	if destinationAccountInfo != nil {
		intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount = destinationAccountInfo.OwnerAccount
	} else if typedProtoMetadata.IsWithdrawal && typedProtoMetadata.DestinationOwner != nil {
		destinationOwner, err := common.NewAccountFromProto(typedProtoMetadata.DestinationOwner)
		if err != nil {
			return err
		}
		intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount = destinationOwner.PublicKey().ToBase58()
	}

	return nil
}

func (h *SendPublicPaymentIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *SendPublicPaymentIntentHandler) GetAccountsWithBalancesToLock(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*common.Account, error) {
	typedMetadata := metadata.GetSendPublicPayment()

	sourceVault, err := common.NewAccountFromProto(typedMetadata.Source)
	if err != nil {
		return nil, err
	}

	return []*common.Account{sourceVault}, nil
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
	// Part 1: Antispam guard checks against the owner
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
	// Part 2: AML checks against the owner
	//

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
	// Part 4: Exchange data validation
	//

	if err := validateExchangeDataWithinIntent(ctx, h.data, typedMetadata.ExchangeData); err != nil {
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

	err = validateFeePayments(ctx, h.data, h.conf, intentRecord, simResult)
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

func (h *SendPublicPaymentIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
	initiatorAccountsByType map[commonpb.AccountType][]*common.AccountRecords,
	initiatorAccountsByVault map[string]*common.AccountRecords,
	metadata *transactionpb.SendPublicPaymentMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	source, err := common.NewAccountFromProto(metadata.Source)
	if err != nil {
		return err
	}

	destination, err := common.NewAccountFromProto(metadata.Destination)
	if err != nil {
		return err
	}

	//
	// Part 1: High-level action validation based on intent metadata
	//

	if metadata.IsRemoteSend && metadata.IsWithdrawal {
		return newIntentValidationError("remote send cannot be a withdraw")
	}

	if !metadata.IsWithdrawal && !metadata.IsRemoteSend && len(actions) != 1 {
		return newIntentValidationError("expected 1 action for payment")
	}
	if metadata.IsWithdrawal && len(actions) != 1 && len(actions) != 2 {
		return newIntentValidationError("expected 1 or 2 actions for withdrawal")
	}
	if metadata.IsRemoteSend && len(actions) != 3 {
		return newIntentValidationError("expected 3 actions for remote send")
	}

	//
	// Part 2: Check the source and destination accounts are valid
	//

	sourceAccountRecords, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
	if !ok || sourceAccountRecords.General.AccountType != commonpb.AccountType_PRIMARY {
		return newIntentValidationError("source account must be a deposit account")
	}

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

		// Fee payments are not required for Code->Code public withdraws
		if metadata.IsWithdrawal && simResult.HasAnyFeePayments() {
			return newIntentValidationErrorf("%s fee payment not required for code destination", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL.String())
		}

		// And the destination cannot be the source of funds, since that results in a no-op
		if source.PublicKey().ToBase58() == destinationAccountInfo.TokenAccount {
			return newIntentValidationError("payment is a no-op")
		}
	case account.ErrAccountInfoNotFound:
		err = func() error {
			// Destination is to a brand new gift card that will be created as part of this
			// intent
			if metadata.IsRemoteSend {
				return nil
			}

			// All payments to external destinations must be withdraws
			if !metadata.IsWithdrawal {
				return newIntentValidationError("payments to external destinations must be withdrawals")
			}

			// Ensure the destination is the core mint ATA for the client-provided owner,
			// if provided. We'll check later if this is absolutely required.
			if metadata.DestinationOwner != nil {
				destinationOwner, err := common.NewAccountFromProto(metadata.DestinationOwner)
				if err != nil {
					return err
				}

				ata, err := destinationOwner.ToAssociatedTokenAccount(common.CoreMintAccount)
				if err != nil {
					return err
				}

				if ata.PublicKey().ToBase58() != destination.PublicKey().ToBase58() {
					return newIntentValidationErrorf("destination is not the ata for %s", destinationOwner.PublicKey().ToBase58())
				}
			}

			// Technically we should always enforce a fee payment, but these checks are only
			// disabled for tests
			if h.conf.disableBlockchainChecks.Get(ctx) {
				return nil
			}

			// Check whether the destination account is a core mint token account that's
			// been created on the blockchain. If not, a fee is required
			err = validateExternalTokenAccountWithinIntent(ctx, h.data, destination)
			switch err {
			case nil:
			default:
				if !strings.Contains(strings.ToLower(err.Error()), "doesn't exist on the blockchain") {
					return err
				}

				if !simResult.HasAnyFeePayments() {
					return newIntentValidationErrorf("%s fee payment is required", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL.String())
				}

				if metadata.DestinationOwner == nil {
					return newIntentValidationError("destination owner account is required to derive ata")
				}
			}

			return nil
		}()
		if err != nil {
			return err
		}
	default:
		return err
	}

	//
	// Part 3 Validate actions match intent metadata
	//

	//
	// Part 3.1: Check destination account is paid exact quark amount from the deposit account
	//           minus any fees
	//

	expectedDestinationPayment := int64(metadata.ExchangeData.Quarks)

	// Minimal validation required here since validateFeePayments generically handles
	// most checks that isn't specific to an intent.
	feePayments := simResult.GetFeePayments()
	if len(feePayments) > 1 {
		return newIntentValidationError("expected at most 1 fee payment")
	}
	for _, feePayment := range feePayments {
		expectedDestinationPayment += feePayment.DeltaQuarks
	}

	destinationSimulation, ok := simResult.SimulationsByAccount[destination.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment to destination account %s", destination.PublicKey().ToBase58())
	} else if destinationSimulation.Transfers[0].IsPrivate || destinationSimulation.Transfers[0].IsWithdraw {
		return newActionValidationError(destinationSimulation.Transfers[0].Action, "payment sent to destination must be a public transfer")
	} else if destinationSimulation.GetDeltaQuarks(false) != expectedDestinationPayment {
		return newActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", expectedDestinationPayment)
	}

	//
	// Part 3.2: Check that the user's deposit account was used as the source of funds
	//           as specified in the metadata
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return newIntentValidationErrorf("must send payment from source account %s", source.PublicKey().ToBase58())
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(metadata.ExchangeData.Quarks) {
		return newActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must send %d quarks from source account", metadata.ExchangeData.Quarks)
	}

	// Part 4: Generic validation of actions that move money

	err = validateMoneyMovementActionUserAccounts(intent.SendPublicPayment, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	// Part 5: Validate open and closed accounts

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
	amlGuard      *aml.Guard

	cachedGiftCardIssuedIntentRecord *intent.Record
}

func NewReceivePaymentsPubliclyIntentHandler(
	conf *conf,
	data code_data.Provider,
	antispamGuard *antispam.Guard,
	amlGuard *aml.Guard,
) CreateIntentHandler {
	return &ReceivePaymentsPubliclyIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,
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

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, currency_util.GetLatestExchangeRateTime())
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
		IsIssuerVoidingGiftCard: false,

		OriginalExchangeCurrency: giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency,
		OriginalExchangeRate:     giftCardIssuedIntentRecord.SendPublicPaymentMetadata.ExchangeRate,
		OriginalNativeAmount:     giftCardIssuedIntentRecord.SendPublicPaymentMetadata.NativeAmount,

		UsdMarketValue: usdExchangeRecord.Rate * float64(typedProtoMetadata.Quarks) / float64(common.CoreMintQuarksPerUnit),
	}

	return nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) GetAccountsWithBalancesToLock(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*common.Account, error) {
	giftCardVault, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
	if err != nil {
		return nil, err
	}
	return []*common.Account{giftCardVault}, nil
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
	// Part 1: Antispam guard checks against the owner
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
	// Part 2: AML checks against the owner
	//

	if !h.conf.disableAmlChecks.Get(ctx) {
		allow, err := h.amlGuard.AllowMoneyMovement(ctx, intentRecord)
		if err != nil {
			return err
		} else if !allow {
			return ErrTransactionLimitExceeded
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

	err = validateFeePayments(ctx, h.data, h.conf, intentRecord, simResult)
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

func validateExchangeDataWithinIntent(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) error {
	isValid, message, err := currency_util.ValidateClientExchangeData(ctx, data, proto)
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

func validateFeePayments(
	ctx context.Context,
	data code_data.Provider,
	conf *conf,
	intentRecord *intent.Record,
	simResult *LocalSimulationResult,
) error {
	var isFeeOptional bool
	var expectedFeeType transactionpb.FeePaymentAction_FeeType
	switch intentRecord.IntentType {
	case intent.SendPublicPayment:
		if intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
			isFeeOptional = true
			expectedFeeType = transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL
		}
	}

	if simResult.HasAnyFeePayments() && expectedFeeType == transactionpb.FeePaymentAction_UNKNOWN {
		return newIntentValidationError("intent doesn't require a fee payment")
	}
	if expectedFeeType == transactionpb.FeePaymentAction_UNKNOWN {
		return nil
	}

	if !simResult.HasAnyFeePayments() && !isFeeOptional {
		return newIntentValidationErrorf("expected a %s fee payment", expectedFeeType.String())
	}
	if !simResult.HasAnyFeePayments() && isFeeOptional {
		return nil
	}

	feePayments := simResult.GetFeePayments()
	if len(feePayments) > 1 {
		return newIntentValidationError("expected at most 1 fee payment")
	} else if len(feePayments) == 0 {
		return nil
	}
	feePayment := feePayments[0]

	if feePayment.Action.GetFeePayment().Type != expectedFeeType {
		return newActionValidationErrorf(feePayment.Action, "expected a %s fee payment", expectedFeeType.String())
	}

	var expectedUsdValue float64
	switch expectedFeeType {
	case transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL:
		expectedUsdValue = conf.createOnSendWithdrawalUsdFee.Get(ctx)
	default:
		return errors.New("unhandled fee type")
	}

	feeAmount := feePayment.DeltaQuarks
	if feeAmount >= 0 {
		return newActionValidationError(feePayment.Action, "fee payment amount is negative")
	}
	feeAmount = -feeAmount // Because it's coming out of a user account in this simulation

	var foundUsdExchangeRecord bool
	usdExchangeRecords, err := currency_util.GetPotentialClientExchangeRates(ctx, data, currency_lib.USD)
	if err != nil {
		return err
	}
	for _, exchangeRecord := range usdExchangeRecords {
		usdValue := exchangeRecord.Rate * float64(feeAmount) / float64(common.CoreMintQuarksPerUnit)

		// Allow for some small margin of error
		if usdValue > expectedUsdValue-0.0001 && usdValue < expectedUsdValue+0.0001 {
			foundUsdExchangeRecord = true
			break
		}
	}

	if !foundUsdExchangeRecord {
		return newActionValidationErrorf(feePayment.Action, "%s fee payment amount must be $%.2f USD", expectedFeeType.String(), expectedUsdValue)
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

	if time.Since(accountInfoRecord.CreatedAt) >= async_account.GiftCardExpiry-time.Minute {
		return newStaleStateError("gift card is expired")
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
