package balance

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/metrics"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

const (
	metricsPackageName = "balance"
)

var (
	// ErrNegativeBalance indicates that a balance calculation resulted in a
	// negative value.
	ErrNegativeBalance = errors.New("balance calculation resulted in negative value")

	// ErrNotManagedByCode indicates that an account is not owned by Code.
	// It's up to callers to determine how to handle this situation within
	// the context of a balance.
	ErrNotManagedByCode = errors.New("explicitly not handling account not managed by code")

	// ErrUnhandledAccount indicates that the balance calculator does not
	// have strategies to handle the provided account.
	ErrUnhandledAccount = errors.New("unhandled account")
)

// Calculator is a function that calculates a token account's balance
type Calculator func(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (uint64, error)

type Strategy func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error)

type State struct {
	// We allow for negative balances in intermediary steps. This is to simplify
	// coordination between strategies. In the end, the sum of all strategies must
	// reflect an accurate picture of the balance, at which point we'll enforce this
	// is positive.
	current int64
}

// DefaultCalculation is the default and recommended strategy for reliably
// estimating a token account's balance.
func DefaultCalculation(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "DefaultCalculation")
	tracer.AddAttribute("account", tokenAccount.PublicKey().ToBase58())
	defer tracer.End()

	timelockRecord, err := data.GetTimelockByVault(ctx, tokenAccount.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		tracer.OnError(ErrNotManagedByCode)
		return 0, ErrNotManagedByCode
	} else if err != nil {
		tracer.OnError(err)
		return 0, err
	}

	// The strategy uses cached values from the intents system. The account must
	// be managed by Code in order to return accurate values.
	isManagedByCode := common.IsManagedByCode(ctx, timelockRecord)
	if !isManagedByCode {
		tracer.OnError(ErrNotManagedByCode)
		return 0, ErrNotManagedByCode
	}

	// Pick a set of strategies relevant for the type of account, so we can optimize
	// the number of DB calls.
	//
	// Overall, we're using a simple strategy that iterates over an account's history
	// to unblock a scheduler implementation optimized for privacy.
	//
	// todo: Come up with a heurisitc that enables some form of checkpointing, so
	//       we're not iterating over all records every time.
	strategies := []Strategy{
		FundingFromExternalDeposits(ctx, data),
		NetBalanceFromIntentActions(ctx, data),
	}
	if timelockRecord.DataVersion == timelock_token.DataVersionLegacy {
		strategies = []Strategy{
			FundingFromExternalDepositsForPrePrivacy2022Accounts(ctx, data),
			NetBalanceFromPrePrivacy2022Intents(ctx, data),
		}
	}

	balance, err := Calculate(
		ctx,
		tokenAccount,
		0,
		strategies...,
	)
	if err != nil {
		tracer.OnError(err)
		return 0, errors.Wrap(err, "error calculating token account balance")
	}
	return balance, nil
}

// Calculate calculates a token account's balance using a starting point and a set
// of strategies. Each may be incomplete individually, but in total must form a
// complete balance calculation.
func Calculate(ctx context.Context, tokenAccount *common.Account, initialBalance uint64, strategies ...Strategy) (balance uint64, err error) {
	balanceState := &State{
		current: int64(initialBalance),
	}

	for _, strategy := range strategies {
		balanceState, err = strategy(ctx, tokenAccount, balanceState)
		if err != nil {
			return 0, err
		}
	}

	if balanceState.current < 0 {
		return 0, ErrNegativeBalance
	}

	return uint64(balanceState.current), nil
}

// NetBalanceFromIntentActions is a balance calculation strategy that incorporates
// the net balance by applying payment intents to the current balance.
func NetBalanceFromIntentActions(ctx context.Context, data code_data.Provider) Strategy {
	return func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error) {
		log := logrus.StandardLogger().WithFields(logrus.Fields{
			"method":  "NetBalanceFromIntentActions",
			"account": tokenAccount.PublicKey().ToBase58(),
		})

		netBalance, err := data.GetNetBalanceFromActions(ctx, tokenAccount.PublicKey().ToBase58())
		if err != nil {
			log.WithError(err).Warn("failure getting net balance from intent actions")
			return nil, errors.Wrap(err, "error getting net balance from intent actions")
		}

		state.current += netBalance
		return state, nil
	}
}

func NetBalanceFromPrePrivacy2022Intents(ctx context.Context, data code_data.Provider) Strategy {
	return func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error) {
		log := logrus.StandardLogger().WithFields(logrus.Fields{
			"method":  "NetBalanceFromPrePrivacy2022Intents",
			"account": tokenAccount.PublicKey().ToBase58(),
		})

		netBalance, err := data.GetNetBalanceFromPrePrivacy2022Intents(ctx, tokenAccount.PublicKey().ToBase58())
		if err != nil {
			log.WithError(err).Warn("failure getting net balance from pre-privacy intents")
			return nil, errors.Wrap(err, "error getting net balance from pre-privacy intents")
		}

		state.current += netBalance
		return state, nil
	}
}

// FundingFromExternalDeposits is a balance calculation strategy that adds funding
// from deposits from external accounts.
func FundingFromExternalDeposits(ctx context.Context, data code_data.Provider) Strategy {
	return func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error) {
		log := logrus.StandardLogger().WithFields(logrus.Fields{
			"method":  "FundingFromExternalDeposits",
			"account": tokenAccount.PublicKey().ToBase58(),
		})

		amount, err := data.GetTotalExternalDepositedAmountInKin(ctx, tokenAccount.PublicKey().ToBase58())
		if err != nil {
			log.WithError(err).Warn("failure getting external deposit amount")
			return nil, errors.Wrap(err, "error getting external deposit amount")
		}
		state.current += int64(amount)

		return state, nil
	}
}

func FundingFromExternalDepositsForPrePrivacy2022Accounts(ctx context.Context, data code_data.Provider) Strategy {
	return func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error) {
		log := logrus.StandardLogger().WithFields(logrus.Fields{
			"method":  "FundingFromLegacyExternalDeposits",
			"account": tokenAccount.PublicKey().ToBase58(),
		})

		amount, err := data.GetLegacyTotalExternalDepositAmountFromPrePrivacy2022Accounts(ctx, tokenAccount.PublicKey().ToBase58())
		if err != nil {
			log.WithError(err).Warn("failure getting external deposit amount")
			return nil, errors.Wrap(err, "error getting external deposit amount")
		}
		state.current += int64(amount)

		return state, nil
	}
}

// BatchCalculator is a functiona that calculates a batch of accounts' balances
type BatchCalculator func(ctx context.Context, data code_data.Provider, accountRecordsBatch []*common.AccountRecords) (map[string]uint64, error)

type BatchStrategy func(ctx context.Context, tokenAccounts []string, state *BatchState) (*BatchState, error)

type BatchState struct {
	// We allow for negative balances in intermediary steps. This is to simplify
	// coordination between strategies. In the end, the sum of all strategies must
	// reflect an accurate picture of the balance, at which point we'll enforce this
	// is positive.
	current map[string]int64
}

// DefaultBatchCalculationWithAccountRecords is the default and recommended batch strategy
// or reliably estimating a set of token accounts' balance when common.AccountRecords are
// available.
//
// Note: This only supports post-privacy accounts. Use DefaultCalculation instead.
func DefaultBatchCalculationWithAccountRecords(ctx context.Context, data code_data.Provider, accountRecordsBatch ...*common.AccountRecords) (map[string]uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "DefaultBatchCalculationWithAccountRecords")
	defer tracer.End()

	timelockRecords := make([]*timelock.Record, len(accountRecordsBatch))
	for i, accountRecords := range accountRecordsBatch {
		timelockRecords[i] = accountRecords.Timelock
	}

	balanceByTokenAccount, err := defaultBatchCalculation(ctx, data, timelockRecords)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}
	return balanceByTokenAccount, nil
}

// DefaultBatchCalculationWithTokenAccounts is the default and recommended batch strategy
// or reliably estimating a set of token accounts' balance when common.Account are
// available.
//
// Note: This only supports post-privacy accounts. Use DefaultCalculation instead.
func DefaultBatchCalculationWithTokenAccounts(ctx context.Context, data code_data.Provider, tokenAccounts ...*common.Account) (map[string]uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "DefaultBatchCalculationWithTokenAccounts")
	defer tracer.End()

	tokenAccountStrings := make([]string, len(tokenAccounts))
	for i, tokenAccount := range tokenAccounts {
		tokenAccountStrings[i] = tokenAccount.PublicKey().ToBase58()
	}

	timelockRecordsByVault, err := data.GetTimelockByVaultBatch(ctx, tokenAccountStrings...)
	if err == timelock.ErrTimelockNotFound {
		tracer.OnError(ErrNotManagedByCode)
		return nil, ErrNotManagedByCode
	} else if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	timelockRecords := make([]*timelock.Record, 0, len(timelockRecordsByVault))
	for _, timelockRecord := range timelockRecordsByVault {
		timelockRecords = append(timelockRecords, timelockRecord)
	}

	balanceByTokenAccount, err := defaultBatchCalculation(ctx, data, timelockRecords)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}
	return balanceByTokenAccount, nil
}

func defaultBatchCalculation(ctx context.Context, data code_data.Provider, timelockRecords []*timelock.Record) (map[string]uint64, error) {
	var tokenAccounts []string
	for _, timelockRecord := range timelockRecords {
		// The strategy uses cached values from the intents system. The account must
		// be managed by Code in order to return accurate values.
		isManagedByCode := common.IsManagedByCode(ctx, timelockRecord)
		if !isManagedByCode {
			return nil, ErrNotManagedByCode
		}

		// We only support post-privacy accounts
		switch timelockRecord.DataVersion {
		case timelock_token.DataVersion1:
			tokenAccounts = append(tokenAccounts, timelockRecord.VaultAddress)
		default:
			return nil, ErrUnhandledAccount
		}
	}

	return CalculateBatch(
		ctx,
		tokenAccounts,
		FundingFromExternalDepositsBatch(ctx, data),
		NetBalanceFromIntentActionsBatch(ctx, data),
	)
}

// Calculate calculates a token account's balance using a starting point and a set
// of strategies. Each may be incomplete individually, but in total must form a
// complete balance calculation.
func CalculateBatch(ctx context.Context, tokenAccounts []string, strategies ...BatchStrategy) (balanceByTokenAccount map[string]uint64, err error) {
	balanceState := &BatchState{
		current: make(map[string]int64),
	}

	for _, strategy := range strategies {
		balanceState, err = strategy(ctx, tokenAccounts, balanceState)
		if err != nil {
			return nil, err
		}
	}

	res := make(map[string]uint64)
	for tokenAccount, balance := range balanceState.current {
		if balance < 0 {
			return nil, ErrNegativeBalance
		}

		res[tokenAccount] = uint64(balance)
	}

	return res, nil
}

// NetBalanceFromIntentActionsBatch is a balance calculation strategy that incorporates
// the net balance by applying payment intents to the current balance.
func NetBalanceFromIntentActionsBatch(ctx context.Context, data code_data.Provider) BatchStrategy {
	return func(ctx context.Context, tokenAccounts []string, state *BatchState) (*BatchState, error) {
		log := logrus.StandardLogger().WithField("method", "NetBalanceFromIntentActionsBatch")

		netBalanceByAccount, err := data.GetNetBalanceFromActionsBatch(ctx, tokenAccounts...)
		if err != nil {
			log.WithError(err).Warn("failure getting net balance from intent actions")
			return nil, errors.Wrap(err, "error getting net balance from intent actions")
		}

		for tokenAccount, netBalance := range netBalanceByAccount {
			state.current[tokenAccount] += netBalance
		}

		return state, nil
	}
}

// FundingFromExternalDepositsBatch is a balance calculation strategy that adds
// funding from deposits from external accounts.
func FundingFromExternalDepositsBatch(ctx context.Context, data code_data.Provider) BatchStrategy {
	return func(ctx context.Context, tokenAccounts []string, state *BatchState) (*BatchState, error) {
		log := logrus.StandardLogger().WithField("method", "FundingFromExternalDepositsBatch")

		amountByAccount, err := data.GetTotalExternalDepositedAmountInKinBatch(ctx, tokenAccounts...)
		if err != nil {
			log.WithError(err).Warn("failure getting external deposit amount")
			return nil, errors.Wrap(err, "error getting external deposit amount")
		}

		for tokenAccount, amount := range amountByAccount {
			state.current[tokenAccount] += int64(amount)
		}

		return state, nil
	}
}

// GetTotalBalance gets an owner account's total balance
//
// todo: consolidate common logic with GetPrivateBalance
func GetTotalBalance(ctx context.Context, data code_data.Provider, owner *common.Account) (uint64, error) {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "GetTotalBalance",
		"owner":  owner.PublicKey().ToBase58(),
	})

	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "GetTotalBalance")
	tracer.AddAttribute("owner", owner.PublicKey().ToBase58())
	defer tracer.End()

	accountRecordsByType, err := common.GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting latest token account records")
		tracer.OnError(err)
		return 0, err
	}

	if len(accountRecordsByType) == 0 {
		tracer.OnError(ErrNotManagedByCode)
		return 0, ErrNotManagedByCode
	}

	var accountRecordsBatch []*common.AccountRecords
	for _, accountRecords := range accountRecordsByType {
		accountRecordsBatch = append(accountRecordsBatch, accountRecords)
	}

	balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(ctx, data, accountRecordsBatch...)
	if err != nil {
		log.WithError(err).Warn("failure getting balances")
		tracer.OnError(err)
		return 0, err
	}

	var total uint64
	for _, records := range accountRecordsByType {
		total += balanceByAccount[records.General.TokenAccount]
	}
	return total, nil
}

// GetPrivateBalance gets an owner account's total private balance (ie. everything
// except the primary account).
//
// todo: consolidate common logic with GetTotalBalance
func GetPrivateBalance(ctx context.Context, data code_data.Provider, owner *common.Account) (uint64, error) {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "GetPrivateBalance",
		"owner":  owner.PublicKey().ToBase58(),
	})

	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "GetPrivateBalance")
	tracer.AddAttribute("owner", owner.PublicKey().ToBase58())
	defer tracer.End()

	accountRecordsByType, err := common.GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting latest token account records")
		tracer.OnError(err)
		return 0, err
	}

	if len(accountRecordsByType) == 0 {
		tracer.OnError(ErrNotManagedByCode)
		return 0, ErrNotManagedByCode
	}

	var accountRecordsBatch []*common.AccountRecords
	for _, accountRecords := range accountRecordsByType {
		switch accountRecords.General.AccountType {
		case commonpb.AccountType_PRIMARY, commonpb.AccountType_LEGACY_PRIMARY_2022, commonpb.AccountType_REMOTE_SEND_GIFT_CARD:
			continue
		}

		accountRecordsBatch = append(accountRecordsBatch, accountRecords)
	}

	balanceByAccount, err := DefaultBatchCalculationWithAccountRecords(ctx, data, accountRecordsBatch...)
	if err != nil {
		log.WithError(err).Warn("failure getting balances")
		tracer.OnError(err)
		return 0, err
	}

	var total uint64
	for _, records := range accountRecordsByType {
		total += balanceByAccount[records.General.TokenAccount]
	}
	return total, nil
}
