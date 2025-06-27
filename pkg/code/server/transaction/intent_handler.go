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

type intentBalanceLock struct {
	// The account that's being locked
	Account *common.Account

	// The function executed on intent DB commit that is guaranteed to prevent
	// race conditions against invalid balance updates
	CommitFn func(ctx context.Context, data code_data.Provider) error
}

// CreateIntentHandler is an interface for handling new intent creations
type CreateIntentHandler interface {
	// PopulateMetadata adds intent metadata to the provided intent record
	// using the client-provided protobuf variant. No other fields in the
	// intent should be modified.
	PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error

	// CreatesNewUser returns whether the intent creates a new Code user identified
	// via a new owner account
	CreatesNewUser(ctx context.Context, metadata *transactionpb.Metadata) (bool, error)

	// IsNoop determines whether the intent is a no-op operation. SubmitIntent will
	// simply return OK and stop any further intent processing.
	//
	// Note: This occurs before validation, so if anything appears out-of-order, then
	// the recommendation is to return false and have the verification logic catch the
	// error.
	IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error)

	// GetBalanceLocks gets a set of global balance locks to prevent race conditions
	// against invalid balance updates that would result in intent fulfillment failure
	GetBalanceLocks(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*intentBalanceLock, error)

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

func (h *OpenAccountsIntentHandler) CreatesNewUser(ctx context.Context, metadata *transactionpb.Metadata) (bool, error) {
	typedMetadata := metadata.GetOpenAccounts()
	if typedMetadata == nil {
		return false, errors.New("unexpected metadata proto message")
	}

	return typedMetadata.AccountSet == transactionpb.OpenAccountsMetadata_USER, nil
}

func (h *OpenAccountsIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	typedMetadata := metadata.GetOpenAccounts()
	if typedMetadata == nil {
		return false, errors.New("unexpected metadata proto message")
	}

	openAction := actions[0].GetOpenAccount()
	if openAction == nil {
		return false, NewActionValidationError(actions[0], "expected an open account action")
	}

	var authorityToCheck *common.Account
	var err error
	switch typedMetadata.AccountSet {
	case transactionpb.OpenAccountsMetadata_USER:
		authorityToCheck, err = common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
		if err != nil {
			return false, err
		}
	case transactionpb.OpenAccountsMetadata_POOL:
		authorityToCheck, err = common.NewAccountFromProto(actions[0].GetOpenAccount().Authority)
		if err != nil {
			return false, err
		}
	default:
		return false, NewIntentValidationErrorf("unsupported account set: %s", typedMetadata.AccountSet)
	}

	_, err = h.data.GetAccountInfoByAuthorityAddress(ctx, authorityToCheck.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (h *OpenAccountsIntentHandler) GetBalanceLocks(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*intentBalanceLock, error) {
	return nil, nil
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
		allow, err := h.antispamGuard.AllowOpenAccounts(ctx, initiatiorOwnerAccount, typedMetadata.AccountSet)
		if err != nil {
			return err
		} else if !allow {
			return NewIntentDeniedError("antispam guard denied account creation")
		}
	}

	//
	// Part 2: Validate the individual actions
	//

	err = h.validateActions(ctx, initiatiorOwnerAccount, typedMetadata, actions)
	if err != nil {
		return err
	}

	//
	// Part 3: Local simulation
	//

	simResult, err := LocalSimulation(ctx, h.data, actions)
	if err != nil {
		return err
	}

	//
	// Part 4: Validate fee payments
	//

	return validateFeePayments(ctx, h.data, h.conf, intentRecord, simResult)
}

func (h *OpenAccountsIntentHandler) validateActions(
	ctx context.Context,
	initiatiorOwnerAccount *common.Account,
	typedMetadata *transactionpb.OpenAccountsMetadata,
	actions []*transactionpb.Action,
) error {
	type expectedAccountToOpen struct {
		Type  commonpb.AccountType
		Index uint64
	}

	var expectedAccountsToOpen []expectedAccountToOpen
	switch typedMetadata.AccountSet {
	case transactionpb.OpenAccountsMetadata_USER:
		expectedAccountsToOpen = []expectedAccountToOpen{
			{
				Type:  commonpb.AccountType_PRIMARY,
				Index: 0,
			},
		}
	case transactionpb.OpenAccountsMetadata_POOL:
		var nextPoolIndex uint64
		latestPoolAccountInfoRecord, err := h.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, initiatiorOwnerAccount.PublicKey().ToBase58(), commonpb.AccountType_POOL)
		switch err {
		case nil:
			nextPoolIndex = latestPoolAccountInfoRecord.Index + 1
		case account.ErrAccountInfoNotFound:
			nextPoolIndex = 0
		default:
			return err
		}

		expectedAccountsToOpen = []expectedAccountToOpen{
			{
				Type:  commonpb.AccountType_POOL,
				Index: nextPoolIndex,
			},
		}
	default:
		return NewIntentValidationErrorf("unsupported account set: %s", typedMetadata.AccountSet)
	}

	expectedActionCount := len(expectedAccountsToOpen)
	if len(actions) != expectedActionCount {
		return NewIntentValidationErrorf("expected %d total actions", expectedActionCount)
	}

	for i, expectedAccountToOpen := range expectedAccountsToOpen {
		openAction := actions[i]

		if openAction.GetOpenAccount() == nil {
			return NewActionValidationError(openAction, "expected an open account action")
		}

		if openAction.GetOpenAccount().AccountType != expectedAccountToOpen.Type {
			return NewActionValidationErrorf(openAction, "account type must be %s", expectedAccountToOpen.Type)
		}

		if openAction.GetOpenAccount().Index != 0 {
			return NewActionValidationErrorf(openAction, "index must be %d", expectedAccountToOpen.Index)
		}

		if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatiorOwnerAccount.PublicKey().ToBytes()) {
			return NewActionValidationErrorf(openAction, "owner must be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
		}

		switch expectedAccountToOpen.Type {
		case commonpb.AccountType_PRIMARY:
			if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
				return NewActionValidationErrorf(openAction, "authority must be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
			}
		default:
			if bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
				return NewActionValidationErrorf(openAction, "authority cannot be %s", initiatiorOwnerAccount.PublicKey().ToBase58())
			}
		}

		expectedVaultAccount, err := getExpectedTimelockVaultFromProtoAccount(openAction.GetOpenAccount().Authority)
		if err != nil {
			return err
		}

		if !bytes.Equal(openAction.GetOpenAccount().Token.Value, expectedVaultAccount.PublicKey().ToBytes()) {
			return NewActionValidationErrorf(openAction, "token must be %s", expectedVaultAccount.PublicKey().ToBase58())
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

	cachedDestinationAccountInfoRecord *account.Record
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
	h.cachedDestinationAccountInfoRecord = destinationAccountInfo

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

func (h *SendPublicPaymentIntentHandler) CreatesNewUser(ctx context.Context, metadata *transactionpb.Metadata) (bool, error) {
	return false, nil
}

func (h *SendPublicPaymentIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *SendPublicPaymentIntentHandler) GetBalanceLocks(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*intentBalanceLock, error) {
	typedMetadata := metadata.GetSendPublicPayment()
	if typedMetadata == nil {
		return nil, errors.New("unexpected metadata proto message")
	}

	sourceVault, err := common.NewAccountFromProto(typedMetadata.Source)
	if err != nil {
		return nil, err
	}

	outgoingSourceBalanceLock, err := balance.GetOptimisticVersionLock(ctx, h.data, sourceVault)
	if err != nil {
		return nil, err
	}

	intentBalanceLocks := []*intentBalanceLock{
		{
			Account:  sourceVault,
			CommitFn: outgoingSourceBalanceLock.OnNewBalanceVersion,
		},
	}

	if h.cachedDestinationAccountInfoRecord != nil {
		switch h.cachedDestinationAccountInfoRecord.AccountType {
		case commonpb.AccountType_POOL:
			closeableDestinationVault, err := common.NewAccountFromProto(typedMetadata.Destination)
			if err != nil {
				return nil, err
			}

			incomingDestinationBalanceLock, err := balance.GetOptimisticVersionLock(ctx, h.data, closeableDestinationVault)
			if err != nil {
				return nil, err
			}
			incomingDestinationOpenCloseLock := balance.NewOpenCloseStatusLock(closeableDestinationVault)

			intentBalanceLocks = append(
				intentBalanceLocks,
				&intentBalanceLock{
					Account:  closeableDestinationVault,
					CommitFn: incomingDestinationBalanceLock.RequireSameBalanceVerion,
				},
				&intentBalanceLock{
					Account:  closeableDestinationVault,
					CommitFn: incomingDestinationOpenCloseLock.OnPaymentToAccount,
				},
			)
		}
	}

	return intentBalanceLocks, nil
}

// todo: validation against Flipcash through generic interface for bet creation
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
		initiatorAccountsByVault,
		typedMetadata,
		actions,
		simResult,
	)
}

func (h *SendPublicPaymentIntentHandler) validateActions(
	ctx context.Context,
	initiatorOwnerAccount *common.Account,
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
		return NewIntentValidationError("remote send cannot be a withdraw")
	}

	if !metadata.IsWithdrawal && !metadata.IsRemoteSend && len(actions) != 1 {
		return NewIntentValidationError("expected 1 action for payment")
	}
	if metadata.IsWithdrawal && len(actions) != 1 && len(actions) != 2 {
		return NewIntentValidationError("expected 1 or 2 actions for withdrawal")
	}
	if metadata.IsRemoteSend && len(actions) != 3 {
		return NewIntentValidationError("expected 3 actions for remote send")
	}

	//
	// Part 2: Check the source and destination accounts are valid
	//

	sourceAccountRecords, ok := initiatorAccountsByVault[source.PublicKey().ToBase58()]
	if !ok || sourceAccountRecords.General.AccountType != commonpb.AccountType_PRIMARY {
		return NewIntentValidationError("source account must be a deposit account")
	}

	if h.cachedDestinationAccountInfoRecord != nil {
		// Remote sends must be to a brand new gift card account
		if metadata.IsRemoteSend {
			return NewIntentValidationError("destination must be a brand new gift card account")
		}

		// Code->Code public ayments can only be made to primary or pool accounts
		switch h.cachedDestinationAccountInfoRecord.AccountType {
		case commonpb.AccountType_PRIMARY, commonpb.AccountType_POOL:
		default:
			return NewIntentValidationError("destination account must be a PRIMARY or POOL account")
		}

		// Code->Code public withdraws must be done against other deposit accounts
		if metadata.IsWithdrawal && h.cachedDestinationAccountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
			return NewIntentValidationError("destination account must be a PRIMARY account")
		}

		// Fee payments are not required for Code->Code public withdraws
		if metadata.IsWithdrawal && simResult.HasAnyFeePayments() {
			return NewIntentValidationErrorf("%s fee payment not required for code destination", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL.String())
		}

		// And the destination cannot be the source of funds, since that results in a no-op
		if source.PublicKey().ToBase58() == h.cachedDestinationAccountInfoRecord.TokenAccount {
			return NewIntentValidationError("payment is a no-op")
		}
	} else {
		err = func() error {
			// Destination is to a brand new gift card that will be created as part of this
			// intent
			if metadata.IsRemoteSend {
				return nil
			}

			// All payments to external destinations must be withdraws
			if !metadata.IsWithdrawal {
				return NewIntentValidationError("payments to external destinations must be withdrawals")
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
					return NewIntentValidationErrorf("destination is not the ata for %s", destinationOwner.PublicKey().ToBase58())
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
					return NewIntentValidationErrorf("%s fee payment is required", transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL.String())
				}

				if metadata.DestinationOwner == nil {
					return NewIntentValidationError("destination owner account is required to derive ata")
				}
			}

			return nil
		}()
		if err != nil {
			return err
		}
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
		return NewIntentValidationError("expected at most 1 fee payment")
	}
	for _, feePayment := range feePayments {
		expectedDestinationPayment += feePayment.DeltaQuarks
	}

	destinationSimulation, ok := simResult.SimulationsByAccount[destination.PublicKey().ToBase58()]
	if !ok {
		return NewIntentValidationErrorf("must send payment to destination account %s", destination.PublicKey().ToBase58())
	} else if destinationSimulation.Transfers[0].IsPrivate || destinationSimulation.Transfers[0].IsWithdraw {
		return NewActionValidationError(destinationSimulation.Transfers[0].Action, "payment sent to destination must be a public transfer")
	} else if destinationSimulation.GetDeltaQuarks(false) != expectedDestinationPayment {
		return NewActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", expectedDestinationPayment)
	}

	//
	// Part 3.2: Check that the user's deposit account was used as the source of funds
	//           as specified in the metadata
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return NewIntentValidationErrorf("must send payment from source account %s", source.PublicKey().ToBase58())
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(metadata.ExchangeData.Quarks) {
		return NewActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must send %d quarks from source account", metadata.ExchangeData.Quarks)
	}

	// Part 4: Generic validation of actions that move money

	err = validateMoneyMovementActionUserAccounts(intent.SendPublicPayment, initiatorAccountsByVault, actions)
	if err != nil {
		return err
	}

	// Part 5: Validate open and closed accounts

	if metadata.IsRemoteSend {
		if len(simResult.GetOpenedAccounts()) != 1 {
			return NewIntentValidationError("expected 1 account opened")
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
			return NewIntentValidationError("expected no closed accounts outside of auto-returns")
		}
		if len(closedAccounts) != 1 {
			return NewIntentValidationError("expected exactly 1 auto-returned account")
		}

		autoReturns := destinationSimulation.GetAutoReturns()
		if len(autoReturns) != 1 {
			return NewIntentValidationError("expected auto-return for the remote send gift card")
		} else if autoReturns[0].IsPrivate || !autoReturns[0].IsWithdraw {
			return NewActionValidationError(destinationSimulation.Transfers[0].Action, "auto-return must be a public withdraw")
		} else if autoReturns[0].DeltaQuarks != -int64(metadata.ExchangeData.Quarks) {
			return NewActionValidationErrorf(autoReturns[0].Action, "must auto-return %d quarks from remote send gift card", metadata.ExchangeData.Quarks)
		}

		autoReturns = sourceSimulation.GetAutoReturns()
		if len(autoReturns) != 1 {
			return NewIntentValidationError("gift card auto-return balance must go to the source account")
		}
	} else {
		if len(simResult.GetOpenedAccounts()) > 0 {
			return NewIntentValidationError("cannot open any account")
		}

		if len(simResult.GetClosedAccounts()) > 0 {
			return NewIntentValidationError("cannot close any account")
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
		return NewIntentValidationError("source is not a remote send gift card")
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

func (h *ReceivePaymentsPubliclyIntentHandler) CreatesNewUser(ctx context.Context, metadata *transactionpb.Metadata) (bool, error) {
	return false, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) GetBalanceLocks(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*intentBalanceLock, error) {
	giftCardVault, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
	if err != nil {
		return nil, err
	}

	outgoingGiftCardBalanceLock, err := balance.GetOptimisticVersionLock(ctx, h.data, giftCardVault)
	if err != nil {
		return nil, err
	}

	return []*intentBalanceLock{
		{
			Account:  giftCardVault,
			CommitFn: outgoingGiftCardBalanceLock.OnNewBalanceVersion,
		},
	}, nil
}

func (h *ReceivePaymentsPubliclyIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
	typedMetadata := untypedMetadata.GetReceivePaymentsPublicly()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	if !typedMetadata.IsRemoteSend {
		return NewIntentValidationError("only remote send is supported")
	}

	if typedMetadata.ExchangeData != nil {
		return NewIntentValidationError("exchange data cannot be set")
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
		return NewIntentValidationError("expected 1 action")
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
		return NewActionValidationError(actions[0], "must send payment to primary account")
	}

	//
	// Part 2: Validate actions match intent
	//

	//
	// Part 2.1: Check source account pays exact quark amount to destination in a public withdraw
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return NewIntentValidationError("must receive payments from source account")
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(metadata.Quarks) {
		return NewActionValidationErrorf(sourceSimulation.Transfers[0].Action, "must receive %d quarks from source account", metadata.Quarks)
	} else if sourceSimulation.Transfers[0].IsPrivate || !sourceSimulation.Transfers[0].IsWithdraw {
		return NewActionValidationError(sourceSimulation.Transfers[0].Action, "transfer must be a public withdraw")
	}

	//
	// Part 2.2: Check destination account is paid exact quark amount from source account in a public withdraw
	//

	if destinationSimulation.GetDeltaQuarks(false) != int64(metadata.Quarks) {
		return NewActionValidationErrorf(actions[0], "must receive %d quarks to temp incoming account", metadata.Quarks)
	} else if destinationSimulation.Transfers[0].IsPrivate || !destinationSimulation.Transfers[0].IsWithdraw {
		return NewActionValidationError(sourceSimulation.Transfers[0].Action, "transfer must be a public withdraw")
	}

	//
	// Part 3: Validate accounts that are opened and closed
	//

	if len(simResult.GetOpenedAccounts()) > 0 {
		return NewIntentValidationError("cannot open any account")
	}

	closedAccounts := simResult.GetClosedAccounts()
	if len(closedAccounts) != 1 {
		return NewIntentValidationError("must close 1 account")
	} else if closedAccounts[0].TokenAccount.PublicKey().ToBase58() != source.PublicKey().ToBase58() {
		return NewActionValidationError(actions[0], "must close source account")
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

type PublicDistributionIntentHandler struct {
	conf          *conf
	data          code_data.Provider
	antispamGuard *antispam.Guard
	amlGuard      *aml.Guard
}

func NewPublicDistributionIntentHandler(
	conf *conf,
	data code_data.Provider,
	antispamGuard *antispam.Guard,
	amlGuard *aml.Guard,
) CreateIntentHandler {
	return &PublicDistributionIntentHandler{
		conf:          conf,
		data:          data,
		antispamGuard: antispamGuard,
		amlGuard:      amlGuard,
	}
}

func (h *PublicDistributionIntentHandler) PopulateMetadata(ctx context.Context, intentRecord *intent.Record, protoMetadata *transactionpb.Metadata) error {
	typedProtoMetadata := protoMetadata.GetPublicDistribution()
	if typedProtoMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	source, err := common.NewAccountFromPublicKeyBytes(typedProtoMetadata.Source.Value)
	if err != nil {
		return err
	}

	var totalQuarks uint64
	for _, distribution := range typedProtoMetadata.Distributions {
		totalQuarks += distribution.Quarks
	}

	usdExchangeRecord, err := h.data.GetExchangeRate(ctx, currency_lib.USD, currency_util.GetLatestExchangeRateTime())
	if err != nil {
		return errors.Wrap(err, "error getting current usd exchange rate")
	}

	intentRecord.IntentType = intent.PublicDistribution
	intentRecord.PublicDistributionMetadata = &intent.PublicDistributionMetadata{
		Source:         source.PublicKey().ToBase58(),
		Quantity:       totalQuarks,
		UsdMarketValue: usdExchangeRecord.Rate * float64(totalQuarks) / float64(common.CoreMintQuarksPerUnit),
	}

	return nil
}

func (h *PublicDistributionIntentHandler) CreatesNewUser(ctx context.Context, metadata *transactionpb.Metadata) (bool, error) {
	return false, nil
}

func (h *PublicDistributionIntentHandler) IsNoop(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) (bool, error) {
	return false, nil
}

func (h *PublicDistributionIntentHandler) GetBalanceLocks(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata) ([]*intentBalanceLock, error) {
	poolVault, err := common.NewAccountFromPublicKeyString(intentRecord.ReceivePaymentsPubliclyMetadata.Source)
	if err != nil {
		return nil, err
	}

	outgoingPoolBalanceLock, err := balance.GetOptimisticVersionLock(ctx, h.data, poolVault)
	if err != nil {
		return nil, err
	}

	incomingPoolBalanceLock := balance.NewOpenCloseStatusLock(poolVault)

	return []*intentBalanceLock{
		{
			Account:  poolVault,
			CommitFn: outgoingPoolBalanceLock.OnNewBalanceVersion,
		},
		{
			Account:  poolVault,
			CommitFn: incomingPoolBalanceLock.OnClose,
		},
	}, nil
}

// todo: validation against Flipcash through generic interface for pool resolution
func (h *PublicDistributionIntentHandler) AllowCreation(ctx context.Context, intentRecord *intent.Record, untypedMetadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
	typedMetadata := untypedMetadata.GetPublicDistribution()
	if typedMetadata == nil {
		return errors.New("unexpected metadata proto message")
	}

	initiatiorOwnerAccount, err := common.NewAccountFromPublicKeyString(intentRecord.InitiatorOwnerAccount)
	if err != nil {
		return err
	}

	//
	// Part 1: Antispam guard checks against the owner
	//

	if !h.conf.disableAntispamChecks.Get(ctx) {
		allow, err := h.antispamGuard.AllowDistribution(ctx, initiatiorOwnerAccount, true)
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
	// Part 3: Pool account validation
	//

	poolVaultAccount, err := common.NewAccountFromProto(typedMetadata.Source)
	if err != nil {
		return err
	}
	var totalQuarksDistributed uint64
	for _, distribution := range typedMetadata.Distributions {
		totalQuarksDistributed += distribution.Quarks
	}
	err = validateDistributedPool(ctx, h.data, poolVaultAccount, totalQuarksDistributed)
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
	// Part 4: Validate fee payments
	//

	err = validateFeePayments(ctx, h.data, h.conf, intentRecord, simResult)
	if err != nil {
		return err
	}

	//
	// Part 5: Validate actions
	//

	return h.validateActions(ctx, typedMetadata, actions, simResult)
}

func (h *PublicDistributionIntentHandler) validateActions(
	ctx context.Context,
	metadata *transactionpb.PublicDistributionMetadata,
	actions []*transactionpb.Action,
	simResult *LocalSimulationResult,
) error {
	if len(actions) != len(metadata.Distributions) {
		return NewIntentValidationErrorf("expected 1 action per distribution")
	}

	//
	// Part 1: Validate source and destination accounts are valid
	//

	// Note: Already validated to be a pool account elsewhere
	source, err := common.NewAccountFromProto(metadata.Source)
	if err != nil {
		return err
	}

	var destinations []*common.Account
	var totalQuarksDistributed uint64
	destinationSet := map[string]any{}
	for _, distribution := range metadata.Distributions {
		destination, err := common.NewAccountFromProto(distribution.Destination)
		if err != nil {
			return err
		}

		_, ok := destinationSet[destination.PublicKey().ToBase58()]
		if ok {
			return NewIntentValidationErrorf("duplicate destination account detected: %s", destination.PublicKey().ToBase58())
		}
		destinationSet[destination.PublicKey().ToBase58()] = true

		destinationAccountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, destination.PublicKey().ToBase58())
		switch err {
		case nil:
			if destinationAccountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
				return NewIntentValidationErrorf("destination account %s must be a primary account", destination.PublicKey().ToBase58())
			}
		case account.ErrAccountInfoNotFound:
			return NewIntentValidationErrorf("destination account %s is not a code account", destination.PublicKey().ToBase58())
		default:
			return err
		}

		totalQuarksDistributed += distribution.Quarks
		destinations = append(destinations, destination)
	}
	if len(destinations) == 0 {
		return NewIntentValidationError("must distribute to at least one destination")
	}

	//
	// Part 2: Validate actions match intent
	//

	//
	// Part 2.1: Check source account pays exact quark amount to each destination
	//

	sourceSimulation, ok := simResult.SimulationsByAccount[source.PublicKey().ToBase58()]
	if !ok {
		return NewIntentValidationError("must send distributions from source account")
	} else if len(sourceSimulation.Transfers) != len(metadata.Distributions) {
		return NewIntentValidationErrorf("must send %d distributions from source account", len(metadata.Distributions))
	} else if sourceSimulation.GetDeltaQuarks(false) != -int64(totalQuarksDistributed) {
		return NewIntentValidationErrorf("must send %d quarks from source account", totalQuarksDistributed)
	}
	for i, transfer := range sourceSimulation.Transfers {
		expectWithdrawal := i == len(destinations)-1
		if transfer.IsPrivate {
			return NewActionValidationError(transfer.Action, "distribution sent from source must be public")
		} else if expectWithdrawal && !transfer.IsWithdraw {
			return NewActionValidationError(transfer.Action, "distribution sent from source must be a withdrawal")
		} else if !expectWithdrawal && transfer.IsWithdraw {
			return NewActionValidationError(transfer.Action, "distribution sent from source must be a transfer")
		}
	}

	//
	// Part 2.2: Check each destination account is paid exact dstirbution quark amount from source account
	//

	for i, destination := range destinations {
		expectWithdrawal := i == len(destinations)-1
		destinationSimulation, ok := simResult.SimulationsByAccount[destination.PublicKey().ToBase58()]
		if !ok {
			return NewIntentValidationErrorf("must send distribution to destination account %s", destination.PublicKey().ToBase58())
		} else if len(destinationSimulation.Transfers) != 1 {
			return NewIntentValidationErrorf("must send distriubtion to destination account %s in one action", destination.PublicKey().ToBase58())
		} else if destinationSimulation.Transfers[0].IsPrivate {
			return NewActionValidationError(destinationSimulation.Transfers[0].Action, "distribution sent to destination must be public")
		} else if expectWithdrawal && !destinationSimulation.Transfers[0].IsWithdraw {
			return NewActionValidationError(destinationSimulation.Transfers[0].Action, "distribution sent to destination must be a withdrawal")
		} else if !expectWithdrawal && destinationSimulation.Transfers[0].IsWithdraw {
			return NewActionValidationError(destinationSimulation.Transfers[0].Action, "distribution sent to destination must be a transfer")
		} else if destinationSimulation.GetDeltaQuarks(false) != int64(metadata.Distributions[i].Quarks) {
			return NewActionValidationErrorf(destinationSimulation.Transfers[0].Action, "must send %d quarks to destination account", int64(metadata.Distributions[i].Quarks))
		}
	}

	// Part 3: Validate open and closed accounts

	if len(simResult.GetOpenedAccounts()) > 0 {
		return NewIntentValidationError("cannot open any account")
	}

	closedAccounts := simResult.GetClosedAccounts()
	if len(closedAccounts) != 1 {
		return NewIntentValidationError("must close 1 account")
	} else if closedAccounts[0].TokenAccount.PublicKey().ToBase58() != source.PublicKey().ToBase58() {
		return NewIntentValidationError("must close source account")
	} else if closedAccounts[0].CloseAction.GetNoPrivacyWithdraw() == nil {
		return NewActionValidationError(closedAccounts[0].CloseAction, "must close source account with a withdraw")
	} else if closedAccounts[0].IsAutoReturned {
		return NewActionValidationError(closedAccounts[0].CloseAction, "action cannot be an auto-return")
	}

	return nil
}

func (h *PublicDistributionIntentHandler) OnSaveToDB(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

func (h *PublicDistributionIntentHandler) OnCommittedToDB(ctx context.Context, intentRecord *intent.Record) error {
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
				return NewActionValidationError(action, "source account must be a deposit account")
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
					return NewActionValidationError(action, "source account must be the primary account")
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
				return NewActionValidationError(action, "source account must be the primary account")
			}
		default:
			continue
		}

		expectedTimelockVault, err := getExpectedTimelockVaultFromProtoAccount(authority.ToProto())
		if err != nil {
			return err
		} else if !bytes.Equal(expectedTimelockVault.PublicKey().ToBytes(), source.PublicKey().ToBytes()) {
			return NewActionValidationErrorf(action, "authority is invalid")
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
					return NewIntentValidationErrorf("multiple open actions for %s account type", commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
				}

				openAction = action
			}
		}
	}

	if openAction == nil {
		return NewIntentValidationErrorf("open account action for %s account type missing", commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
	}

	if bytes.Equal(openAction.GetOpenAccount().Owner.Value, initiatorOwnerAccount.PublicKey().ToBytes()) {
		return NewActionValidationErrorf(openAction, "owner cannot be %s", initiatorOwnerAccount.PublicKey().ToBase58())
	}

	if !bytes.Equal(openAction.GetOpenAccount().Owner.Value, openAction.GetOpenAccount().Authority.Value) {
		return NewActionValidationErrorf(openAction, "authority must be %s", openAction.GetOpenAccount().Owner.Value)
	}

	if openAction.GetOpenAccount().Index != 0 {
		return NewActionValidationError(openAction, "index must be 0")
	}

	derivedVaultAccount, err := getExpectedTimelockVaultFromProtoAccount(openAction.GetOpenAccount().Authority)
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedGiftCardVault.PublicKey().ToBytes(), derivedVaultAccount.PublicKey().ToBytes()) {
		return NewActionValidationErrorf(openAction, "token must be %s", expectedGiftCardVault.PublicKey().ToBase58())
	}

	if !bytes.Equal(openAction.GetOpenAccount().Token.Value, derivedVaultAccount.PublicKey().ToBytes()) {
		return NewActionValidationErrorf(openAction, "token must be %s", derivedVaultAccount.PublicKey().ToBase58())
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
		return NewIntentValidationError(message)
	}
	return nil
}

func validateExchangeDataWithinIntent(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) error {
	isValid, message, err := currency_util.ValidateClientExchangeData(ctx, data, proto)
	if err != nil {
		return err
	} else if !isValid {
		if strings.Contains(message, "stale") {
			return NewStaleStateError(message)
		}
		return NewIntentValidationError(message)
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
		return NewIntentValidationError("intent doesn't require a fee payment")
	}
	if expectedFeeType == transactionpb.FeePaymentAction_UNKNOWN {
		return nil
	}

	if !simResult.HasAnyFeePayments() && !isFeeOptional {
		return NewIntentValidationErrorf("expected a %s fee payment", expectedFeeType.String())
	}
	if !simResult.HasAnyFeePayments() && isFeeOptional {
		return nil
	}

	feePayments := simResult.GetFeePayments()
	if len(feePayments) > 1 {
		return NewIntentValidationError("expected at most 1 fee payment")
	} else if len(feePayments) == 0 {
		return nil
	}
	feePayment := feePayments[0]

	if feePayment.Action.GetFeePayment().Type != expectedFeeType {
		return NewActionValidationErrorf(feePayment.Action, "expected a %s fee payment", expectedFeeType.String())
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
		return NewActionValidationError(feePayment.Action, "fee payment amount is negative")
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
		return NewActionValidationErrorf(feePayment.Action, "%s fee payment amount must be $%.2f USD", expectedFeeType.String(), expectedUsdValue)
	}

	return nil
}

func validateClaimedGiftCard(ctx context.Context, data code_data.Provider, giftCardVaultAccount *common.Account, claimedAmount uint64) error {
	//
	// Part 1: Is the account a gift card?
	//

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound || accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return NewIntentValidationError("source is not a remote send gift card")
	}

	//
	// Part 2: Is there already an action to claim the gift card balance?
	//

	_, err = data.GetGiftCardClaimedAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == nil {
		return NewStaleStateError("gift card balance has already been claimed")
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
		return NewStaleStateError("gift card is expired")
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
			return NewStaleStateError("gift card balance has already been claimed")
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
		return NewStaleStateError("gift card balance has already been claimed")
	} else if giftCardBalance != claimedAmount {
		return NewIntentValidationErrorf("must receive entire gift card balance of %d quarks", giftCardBalance)
	}

	//
	// Part 6: Are we within the threshold for auto-return back to the issuer?
	//

	if time.Since(accountInfoRecord.CreatedAt) >= async_account.GiftCardExpiry-time.Minute {
		return NewStaleStateError("gift card is expired")
	}

	return nil
}

func validateDistributedPool(ctx context.Context, data code_data.Provider, poolVaultAccount *common.Account, distributedAmount uint64) error {
	//
	// Part 1: Is the account a pool?
	//

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, poolVaultAccount.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound || accountInfoRecord.AccountType != commonpb.AccountType_POOL {
		return NewIntentValidationError("source is not a pool account")
	}

	//
	// Part 2: Is the full amount being distributed?
	//

	poolBalance, err := balance.CalculateFromCache(ctx, data, poolVaultAccount)
	if err != nil {
		return err
	} else if poolBalance == 0 {
		return NewStaleStateError("pool balance has already been distributed")
	} else if distributedAmount != poolBalance {
		return NewIntentValidationErrorf("must distribute entire pool balance of %d quarks", poolBalance)
	}

	//
	// Part 3: Is the pool account managed by Code?
	//

	timelockRecord, err := data.GetTimelockByVault(ctx, poolVaultAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	isManagedByCode := common.IsManagedByCode(ctx, timelockRecord)
	if !isManagedByCode {
		if timelockRecord.IsClosed() {
			return NewStaleStateError("pool balance has already been distributed")
		}
		return ErrNotManagedByCode
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
		return NewIntentDeniedError("an account being opened has already initiated an unlock")
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
