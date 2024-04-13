package common

import (
	"context"
	"errors"
	"sync"

	"github.com/newrelic/go-agent/v3/newrelic"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	// Important Note: Be very careful changing this value, as it will completely
	// change timelock PDAs and have consequences with existing splitter treasuries.
	//
	// todo: configurable
	realSubsidizerPublicKey = "codeHy87wGD5oMRLG75qKqsSi1vWE3oxNyYmXo5F9YR"

	// Ensure this is a large enough buffer. The enforcement of a min balance isn't
	// perfect to say the least.
	//
	// todo: configurable
	minSubsidizerBalance = 250_000_000_000 // 250 SOL
)

var (
	// This doesn't account for recovery of rent, which implies some fulfillments
	// actually have negative fees. We often need to think about "in flight" costs
	// and SOL balances for our subsidizer, so we exclude rent recovery which
	// ensures our estimates are always on the conservative side of things.
	lamportsByFulfillment = map[fulfillment.Type]uint64{
		fulfillment.InitializeLockedTimelockAccount:       4120000,  // 0.00412 SOL
		fulfillment.NoPrivacyTransferWithAuthority:        10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.NoPrivacyWithdraw:                     10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.TemporaryPrivacyTransferWithAuthority: 10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.PermanentPrivacyTransferWithAuthority: 10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.TransferWithCommitment:                5000,     // 0.000005 SOL (5000 lamports per signature)
		fulfillment.CloseEmptyTimelockAccount:             10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.CloseDormantTimelockAccount:           10000,    // 0.00001 SOL (5000 lamports per signature)
		fulfillment.SaveRecentRoot:                        5000,     // 0.000005 SOL (5000 lamports per signature)
		fulfillment.InitializeCommitmentProof:             15800000, // 0.0158 SOL
		fulfillment.UploadCommitmentProof:                 5000,     // 0.000005 SOL (5000 lamports per signature)
		fulfillment.VerifyCommitmentProof:                 5000,     // 0.000005 SOL (5000 lamports per signature)
		fulfillment.OpenCommitmentVault:                   2050000,  // 0.00205 SOL
		fulfillment.CloseCommitmentVault:                  5000,     // 0.000005 SOL (5000 lamports per signature)
	}
	lamportsPerCreateNonceAccount uint64 = 1450000 // 0.00145 SOL
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

	numNoncesBeingCreated, err := data.GetNonceCountByState(ctx, nonce.StateUnknown)
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

	nr, ok := ctx.Value(metrics.NewRelicContextKey{}).(*newrelic.Application)
	if ok {
		nr.RecordCustomMetric("Subsidizer/min_balance_enforced", 1)
	}

	return ErrSubsidizerRequiresFunding
}
