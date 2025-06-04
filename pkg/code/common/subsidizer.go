package common

import (
	"context"
	"errors"
	"sync"

	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/config"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
)

// todo: always assumes mainnet

const (
	// Important Note: Be very careful changing this value, as it will completely
	// change timelock PDAs and have consequences with existing splitter treasuries.
	realSubsidizerPublicKey = config.SubsidizerPublicKey

	// Ensure this is a large enough buffer. The enforcement of a min balance isn't
	// perfect to say the least.
	//
	// todo: configurable
	minSubsidizerBalance = 5_000_000_000 // 5 SOL
)

// todo: doesn't consider external deposits
// todo: need a better system given fees are dynamic, we'll consider the worst case for each fulfillment type to be safe
var (
	lamportsByFulfillment = map[fulfillment.Type]uint64{
		fulfillment.InitializeLockedTimelockAccount: 5050,
		fulfillment.NoPrivacyTransferWithAuthority:  203928 + 5125,
		fulfillment.NoPrivacyWithdraw:               5100,
		fulfillment.CloseEmptyTimelockAccount:       5100,
	}
	lamportsPerCreateNonceAccount uint64 = 1450000
)

var (
	subsidizerAccountLock sync.RWMutex
	subsidizerAccount     *Account

	ErrSubsidizerRequiresFunding = errors.New("subsidizer requires funding")
)

// GetSubsidizer gets the subsidizer account, as initially loaded in LoadSubsidizer
func GetSubsidizer() *Account {
	subsidizerAccountLock.RLock()
	defer subsidizerAccountLock.RUnlock()

	if subsidizerAccount == nil {
		panic("subsidizer wasn't loaded from db")
	}

	copied, err := NewAccountFromPrivateKeyString(subsidizerAccount.PrivateKey().ToBase58())
	if err != nil {
		panic(err)
	}

	return copied
}

// LoadProductionSubsidizer loads the production subsidizer account by it's public
// key over the provided data provider. This should be done exactly once on app launch.
// Use GetSubsidizer to get the Account struct.
func LoadProductionSubsidizer(ctx context.Context, data code_data.Provider) error {
	subsidizerAccountLock.Lock()
	defer subsidizerAccountLock.Unlock()

	if subsidizerAccount != nil {
		if subsidizerAccount.PublicKey().ToBase58() == realSubsidizerPublicKey {
			return nil
		}

		return errors.New("unexpected subsidizer account already loaded")
	}

	vaultRecord, err := data.GetKey(ctx, realSubsidizerPublicKey)
	if err != nil {
		return err
	}

	account, err := NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
	if err != nil {
		return err
	}

	if account.PublicKey().ToBase58() != realSubsidizerPublicKey {
		return errors.New("subsidizer public key mismatch")
	}

	subsidizerAccount = account

	return nil
}

// InjectTestSubsidizer injects a provided account as a subsidizer for testing
// purposes. Do not call this in a production setting.
func InjectTestSubsidizer(ctx context.Context, data code_data.Provider, testAccount *Account) error {
	subsidizerAccountLock.Lock()
	defer subsidizerAccountLock.Unlock()

	if subsidizerAccount != nil && subsidizerAccount.PublicKey().ToBase58() == realSubsidizerPublicKey {
		return errors.New("attempted to inject test subsidizer in production environment")
	}

	subsidizerAccount = testAccount
	return nil
}

// GetCurrentSubsidizerBalance returns the subsidizer's current balance in lamports.
func GetCurrentSubsidizerBalance(ctx context.Context, data code_data.Provider) (uint64, error) {
	accountInfo, err := data.GetBlockchainAccountInfo(ctx, GetSubsidizer().PublicKey().ToBase58(), solana.CommitmentProcessed)
	if err != nil {
		return 0, err
	}
	return accountInfo.Lamports, nil
}

// EstimateUsedSubsidizerBalance estimates the number of lamports that will be used
// by in flight fulfillments.
//
// todo: This assumes fees are constant, which might not be the case in the future.
func EstimateUsedSubsidizerBalance(ctx context.Context, data code_data.Provider) (uint64, error) {
	var fees uint64

	pendingFulfillmentsByType, err := data.GetPendingFulfillmentCountByType(ctx)
	if err != nil {
		return 0, err
	}

	for fulfillmentType, lamportsConsumed := range lamportsByFulfillment {
		count, ok := pendingFulfillmentsByType[fulfillmentType]
		if !ok {
			continue
		}

		fees += uint64(count) * lamportsConsumed
	}

	numNoncesBeingCreated, err := data.GetNonceCountByState(ctx, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.StateUnknown)
	if err != nil {
		return 0, err
	}
	fees += lamportsPerCreateNonceAccount * numNoncesBeingCreated

	return fees, nil
}

// EnforceMinimumSubsidizerBalance returns ErrSubsidizerRequiresFunding if the
// subsidizer's estimated available balance breaks a given threshold.
func EnforceMinimumSubsidizerBalance(ctx context.Context, data code_data.Provider) error {
	balance, err := GetCurrentSubsidizerBalance(ctx, data)
	if err != nil {
		return err
	}

	fees, err := EstimateUsedSubsidizerBalance(ctx, data)
	if err != nil {
		return err
	}

	if fees < balance && balance-fees > minSubsidizerBalance {
		return nil
	}

	nr, ok := ctx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
	if ok {
		nr.RecordCustomMetric("Subsidizer/min_balance_enforced", 1)
	}

	return ErrSubsidizerRequiresFunding
}
