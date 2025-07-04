package balance

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/balance"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
)

type Source uint8

const (
	UnknownSource Source = iota
	CacheSource
	BlockchainSource
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

// CalculateFromCache is the default and recommended strategy for reliably estimating
// a token account's balance using cached values.
//
// Note: Use this method when calculating balances for accounts that are managed by
// Code (ie. Timelock account) and operate within the L2 system.
func CalculateFromCache(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "CalculateFromCache")
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

// CalculateFromBlockchain is the default and recommended strategy for reliably
// estimating a token account's balance from the blockchain. This strategy is
// resistant to various RPC failure nodes, and may return a cached value. The
// source of the balance calculation is returned.
//
// Note: Use this method when calculating token account balances that are external
// and not managed by Code and outside the L2 system.
//
// todo: add a batching variant
func CalculateFromBlockchain(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) (uint64, Source, error) {
	var cachedQuarks uint64
	var cachedSlot uint64
	var cachedUpdateTs time.Time
	checkpointRecord, err := data.GetExternalBalanceCheckpoint(ctx, tokenAccount.PublicKey().ToBase58())
	if err == nil {
		cachedQuarks = checkpointRecord.Quarks
		cachedSlot = checkpointRecord.SlotCheckpoint
		cachedUpdateTs = checkpointRecord.LastUpdatedAt
	} else if err != balance.ErrCheckpointNotFound {
		return 0, UnknownSource, err
	}

	// todo: we may need something that's more resistant to RPC nodes with stale account state
	quarks, slot, err := data.GetBlockchainBalance(ctx, tokenAccount.PublicKey().ToBase58())
	if err == solana.ErrNoBalance {
		// We can't tell whether
		//  1. RPC node is behind, and observed a state before the account existed
		//  2. RPC node is ahead, and the account was closed
		// because we don't have a slot to compare against the checkpoint.
		//
		// If the checkpoint was recently updated, we opt to trust that, optimizing
		// to reduce potential race conditions for 1.
		if time.Since(cachedUpdateTs) < 5*time.Minute {
			return cachedQuarks, CacheSource, nil
		}

		return 0, BlockchainSource, nil
	} else if err != nil {
		// RPC node threw an error. Return the cached balance
		return cachedQuarks, CacheSource, nil
	}

	// RPC node is behind, use cached balance
	if cachedSlot > slot {
		return cachedQuarks, CacheSource, nil
	}

	// Observed a balance that's more recent. Best-effort update the checkpoint.
	if cachedSlot == 0 || (slot > cachedSlot && quarks != cachedQuarks) {
		newCheckpointRecord := &balance.ExternalCheckpointRecord{
			TokenAccount:   tokenAccount.PublicKey().ToBase58(),
			Quarks:         quarks,
			SlotCheckpoint: slot,
		}
		data.SaveExternalBalanceCheckpoint(ctx, newCheckpointRecord)
	}

	return quarks, BlockchainSource, nil
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

// FundingFromExternalDeposits is a balance calculation strategy that adds funding
// from deposits from external accounts.
func FundingFromExternalDeposits(ctx context.Context, data code_data.Provider) Strategy {
	return func(ctx context.Context, tokenAccount *common.Account, state *State) (*State, error) {
		log := logrus.StandardLogger().WithFields(logrus.Fields{
			"method":  "FundingFromExternalDeposits",
			"account": tokenAccount.PublicKey().ToBase58(),
		})

		amount, err := data.GetTotalExternalDepositedAmountInQuarks(ctx, tokenAccount.PublicKey().ToBase58())
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

// CalculateBatch calculates a set of token accounts' balance using a starting point
// and a set of strategies. Each may be incomplete individually, but in total must
// form a complete balance calculation.
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

// BatchCalculateFromCacheWithAccountRecords is the default and recommended batch strategy
// or reliably estimating a set of token accounts' balance when common.AccountRecords are
// available.
//
// Note: Use this method when calculating balances for accounts that are managed by
// Code (ie. Timelock account) and operate within the L2 system.
func BatchCalculateFromCacheWithAccountRecords(ctx context.Context, data code_data.Provider, accountRecordsBatch ...*common.AccountRecords) (map[string]uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "BatchCalculateFromCacheWithAccountRecords")
	defer tracer.End()

	timelockRecords := make([]*timelock.Record, 0)
	for _, accountRecords := range accountRecordsBatch {
		if !accountRecords.IsTimelock() {
			tracer.OnError(ErrNotManagedByCode)
			return nil, ErrNotManagedByCode
		}

		timelockRecords = append(timelockRecords, accountRecords.Timelock)
	}

	balanceByTokenAccount, err := defaultBatchCalculationFromCache(ctx, data, timelockRecords)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}
	return balanceByTokenAccount, nil
}

// BatchCalculateFromCacheWithTokenAccounts is the default and recommended batch strategy
// or reliably estimating a set of token accounts' balance when common.Account are
// available.
//
// Note: Use this method when calculating balances for accounts that are managed by
// Code (ie. Timelock account) and operate within the L2 system.
func BatchCalculateFromCacheWithTokenAccounts(ctx context.Context, data code_data.Provider, tokenAccounts ...*common.Account) (map[string]uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "BatchCalculateFromCacheWithTokenAccounts")
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

	balanceByTokenAccount, err := defaultBatchCalculationFromCache(ctx, data, timelockRecords)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}
	return balanceByTokenAccount, nil
}

func defaultBatchCalculationFromCache(ctx context.Context, data code_data.Provider, timelockRecords []*timelock.Record) (map[string]uint64, error) {
	var tokenAccounts []string
	for _, timelockRecord := range timelockRecords {
		// The strategy uses cached values from the intents system. The account must
		// be managed by Code in order to return accurate values.
		isManagedByCode := common.IsManagedByCode(ctx, timelockRecord)
		if !isManagedByCode {
			return nil, ErrNotManagedByCode
		}

		tokenAccounts = append(tokenAccounts, timelockRecord.VaultAddress)
	}

	return CalculateBatch(
		ctx,
		tokenAccounts,
		FundingFromExternalDepositsBatch(ctx, data),
		NetBalanceFromIntentActionsBatch(ctx, data),
	)
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

		amountByAccount, err := data.GetTotalExternalDepositedAmountInQuarksBatch(ctx, tokenAccounts...)
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

func (s Source) String() string {
	switch s {
	case UnknownSource:
		return "unknown"
	case CacheSource:
		return "cache"
	case BlockchainSource:
		return "blockchain"
	}
	return "unknown"
}
