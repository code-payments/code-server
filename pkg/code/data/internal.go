package data

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/cache"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	pg "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/balance"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	cvm_ram "github.com/code-payments/code-server/pkg/code/data/cvm/ram"
	cvm_storage "github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/messaging"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/vault"

	account_memory_client "github.com/code-payments/code-server/pkg/code/data/account/memory"
	action_memory_client "github.com/code-payments/code-server/pkg/code/data/action/memory"
	balance_memory_client "github.com/code-payments/code-server/pkg/code/data/balance/memory"
	currency_memory_client "github.com/code-payments/code-server/pkg/code/data/currency/memory"
	cvm_ram_memory_client "github.com/code-payments/code-server/pkg/code/data/cvm/ram/memory"
	cvm_storage_memory_client "github.com/code-payments/code-server/pkg/code/data/cvm/storage/memory"
	deposit_memory_client "github.com/code-payments/code-server/pkg/code/data/deposit/memory"
	fulfillment_memory_client "github.com/code-payments/code-server/pkg/code/data/fulfillment/memory"
	intent_memory_client "github.com/code-payments/code-server/pkg/code/data/intent/memory"
	merkletree_memory_client "github.com/code-payments/code-server/pkg/code/data/merkletree/memory"
	messaging_memory_client "github.com/code-payments/code-server/pkg/code/data/messaging/memory"
	nonce_memory_client "github.com/code-payments/code-server/pkg/code/data/nonce/memory"
	rendezvous_memory_client "github.com/code-payments/code-server/pkg/code/data/rendezvous/memory"
	swap_memory_client "github.com/code-payments/code-server/pkg/code/data/swap/memory"
	timelock_memory_client "github.com/code-payments/code-server/pkg/code/data/timelock/memory"
	transaction_memory_client "github.com/code-payments/code-server/pkg/code/data/transaction/memory"
	vault_memory_client "github.com/code-payments/code-server/pkg/code/data/vault/memory"

	account_postgres_client "github.com/code-payments/code-server/pkg/code/data/account/postgres"
	action_postgres_client "github.com/code-payments/code-server/pkg/code/data/action/postgres"
	balance_postgres_client "github.com/code-payments/code-server/pkg/code/data/balance/postgres"
	currency_postgres_client "github.com/code-payments/code-server/pkg/code/data/currency/postgres"
	cvm_ram_postgres_client "github.com/code-payments/code-server/pkg/code/data/cvm/ram/postgres"
	cvm_storage_postgres_client "github.com/code-payments/code-server/pkg/code/data/cvm/storage/postgres"
	deposit_postgres_client "github.com/code-payments/code-server/pkg/code/data/deposit/postgres"
	fulfillment_postgres_client "github.com/code-payments/code-server/pkg/code/data/fulfillment/postgres"
	intent_postgres_client "github.com/code-payments/code-server/pkg/code/data/intent/postgres"
	merkletree_postgres_client "github.com/code-payments/code-server/pkg/code/data/merkletree/postgres"
	messaging_postgres_client "github.com/code-payments/code-server/pkg/code/data/messaging/postgres"
	nonce_postgres_client "github.com/code-payments/code-server/pkg/code/data/nonce/postgres"
	rendezvous_postgres_client "github.com/code-payments/code-server/pkg/code/data/rendezvous/postgres"
	swap_postgres_client "github.com/code-payments/code-server/pkg/code/data/swap/postgres"
	timelock_postgres_client "github.com/code-payments/code-server/pkg/code/data/timelock/postgres"
	transaction_postgres_client "github.com/code-payments/code-server/pkg/code/data/transaction/postgres"
	vault_postgres_client "github.com/code-payments/code-server/pkg/code/data/vault/postgres"
)

// Cache Constants
const (
	maxExchangeRateCacheBudget    = 1000000 // 1 million
	singleExchangeRateCacheWeight = 1
	multiExchangeRateCacheWeight  = 60 // usually we get 60 exchange rates from CoinGecko for a single time interval

	maxTimelockCacheBudget = 100000
	timelockCacheTTL       = 5 * time.Second // Keep this relatively small
)

type timelockCacheEntry struct {
	mu            sync.RWMutex
	record        *timelock.Record
	lastUpdatedAt time.Time
}

type DatabaseData interface {
	// Account Info
	// --------------------------------------------------------------------------------
	CreateAccountInfo(ctx context.Context, record *account.Record) error
	UpdateAccountInfo(ctx context.Context, record *account.Record) error
	GetAccountInfoByTokenAddress(ctx context.Context, address string) (*account.Record, error)
	GetAccountInfoByTokenAddressBatch(ctx context.Context, addresses ...string) (map[string]*account.Record, error)
	GetAccountInfoByAuthorityAddress(ctx context.Context, address string) (map[string]*account.Record, error)
	GetLatestAccountInfosByOwnerAddress(ctx context.Context, address string) (map[string]map[commonpb.AccountType][]*account.Record, error)
	GetLatestAccountInfoByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (map[string]*account.Record, error)
	GetPrioritizedAccountInfosRequiringDepositSync(ctx context.Context, limit uint64) ([]*account.Record, error)
	GetPrioritizedAccountInfosRequiringAutoReturnCheck(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error)
	GetAccountInfoCountRequiringDepositSync(ctx context.Context) (uint64, error)
	GetAccountInfoCountRequiringAutoReturnCheck(ctx context.Context) (uint64, error)

	// Actions
	// --------------------------------------------------------------------------------
	PutAllActions(ctx context.Context, records ...*action.Record) error
	UpdateAction(ctx context.Context, record *action.Record) error
	GetActionById(ctx context.Context, intent string, actionId uint32) (*action.Record, error)
	GetAllActionsByIntent(ctx context.Context, intent string) ([]*action.Record, error)
	GetAllActionsByAddress(ctx context.Context, address string) ([]*action.Record, error)
	GetNetBalanceFromActions(ctx context.Context, address string) (int64, error)
	GetNetBalanceFromActionsBatch(ctx context.Context, accounts ...string) (map[string]int64, error)
	GetGiftCardClaimedAction(ctx context.Context, giftCardVault string) (*action.Record, error)
	GetGiftCardAutoReturnAction(ctx context.Context, giftCardVault string) (*action.Record, error)
	CountFeeActions(ctx context.Context, intent string, feeType transactionpb.FeePaymentAction_FeeType) (uint64, error)
	HasFeeAction(ctx context.Context, intent string, feeType transactionpb.FeePaymentAction_FeeType) (bool, error)

	// Balance
	// --------------------------------------------------------------------------------
	GetCachedBalanceVersion(ctx context.Context, account string) (uint64, error)
	AdvanceCachedBalanceVersion(ctx context.Context, account string, currentVersion uint64) error
	CheckNotClosedForBalanceUpdate(ctx context.Context, account string) error
	MarkAsClosedForBalanceUpdate(ctx context.Context, account string) error
	SaveExternalBalanceCheckpoint(ctx context.Context, record *balance.ExternalCheckpointRecord) error
	GetExternalBalanceCheckpoint(ctx context.Context, account string) (*balance.ExternalCheckpointRecord, error)

	// Currency
	// --------------------------------------------------------------------------------
	GetExchangeRate(ctx context.Context, code currency_lib.Code, t time.Time) (*currency.ExchangeRateRecord, error)
	GetAllExchangeRates(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error)
	GetExchangeRateHistory(ctx context.Context, code currency_lib.Code, opts ...query.Option) ([]*currency.ExchangeRateRecord, error)
	ImportExchangeRates(ctx context.Context, record *currency.MultiRateRecord) error
	PutCurrencyMetadata(ctx context.Context, record *currency.MetadataRecord) error
	GetCurrencyMetadata(ctx context.Context, mint string) (*currency.MetadataRecord, error)
	PutCurrencyReserve(ctx context.Context, record *currency.ReserveRecord) error
	GetCurrencyReserveAtTime(ctx context.Context, mint string, t time.Time) (*currency.ReserveRecord, error)

	// CVM RAM
	// --------------------------------------------------------------------------------
	InitializeVmMemory(ctx context.Context, record *cvm_ram.Record) error
	FreeVmMemoryByIndex(ctx context.Context, memoryAccount string, index uint16) error
	FreeVmMemoryByAddress(ctx context.Context, address string) error
	ReserveVmMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType, address string) (string, uint16, error)

	// CVM Storage
	// --------------------------------------------------------------------------------
	InitializeVmStorage(ctx context.Context, record *cvm_storage.Record) error
	FindAnyVmStorageWithAvailableCapacity(ctx context.Context, vm string, purpose cvm_storage.Purpose, minCapacity uint64) (*cvm_storage.Record, error)
	ReserveVmStorage(ctx context.Context, vm string, purpose cvm_storage.Purpose, address string) (string, error)

	// Deposits
	// --------------------------------------------------------------------------------
	SaveExternalDeposit(ctx context.Context, record *deposit.Record) error
	GetExternalDeposit(ctx context.Context, signature, destination string) (*deposit.Record, error)
	GetTotalExternalDepositedAmountInQuarks(ctx context.Context, account string) (uint64, error)
	GetTotalExternalDepositedAmountInQuarksBatch(ctx context.Context, accounts ...string) (map[string]uint64, error)
	GetTotalExternalDepositedAmountInUsd(ctx context.Context, account string) (float64, error)

	// Fulfillments
	// --------------------------------------------------------------------------------
	GetFulfillmentById(ctx context.Context, id uint64) (*fulfillment.Record, error)
	GetFulfillmentBySignature(ctx context.Context, signature string) (*fulfillment.Record, error)
	GetFulfillmentByVirtualSignature(ctx context.Context, signature string) (*fulfillment.Record, error)
	GetFulfillmentCount(ctx context.Context) (uint64, error)
	GetFulfillmentCountByState(ctx context.Context, state fulfillment.State) (uint64, error)
	GetFulfillmentCountByStateGroupedByType(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error)
	GetFulfillmentCountForMetrics(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error)
	GetFulfillmentCountByStateAndAddress(ctx context.Context, state fulfillment.State, address string) (uint64, error)
	GetFulfillmentCountByTypeStateAndAddress(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error)
	GetFulfillmentCountByTypeStateAndAddressAsSource(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error)
	GetFulfillmentCountByIntentAndState(ctx context.Context, intent string, state fulfillment.State) (uint64, error)
	GetFulfillmentCountByIntent(ctx context.Context, intent string) (uint64, error)
	GetFulfillmentCountByTypeActionAndState(ctx context.Context, intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) (uint64, error)
	GetPendingFulfillmentCountByType(ctx context.Context) (map[fulfillment.Type]uint64, error)
	GetAllFulfillmentsByState(ctx context.Context, state fulfillment.State, includeDisabledActiveScheduling bool, opts ...query.Option) ([]*fulfillment.Record, error)
	GetAllFulfillmentsByIntent(ctx context.Context, intent string, opts ...query.Option) ([]*fulfillment.Record, error)
	GetAllFulfillmentsByAction(ctx context.Context, intentId string, actionId uint32) ([]*fulfillment.Record, error)
	GetAllFulfillmentsByTypeAndAction(ctx context.Context, fulfillmentType fulfillment.Type, intentId string, actionId uint32) ([]*fulfillment.Record, error)
	GetFirstSchedulableFulfillmentByAddressAsSource(ctx context.Context, address string) (*fulfillment.Record, error)
	GetFirstSchedulableFulfillmentByAddressAsDestination(ctx context.Context, address string) (*fulfillment.Record, error)
	GetFirstSchedulableFulfillmentByType(ctx context.Context, fulfillmentType fulfillment.Type) (*fulfillment.Record, error)
	GetNextSchedulableFulfillmentByAddress(ctx context.Context, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*fulfillment.Record, error)
	PutAllFulfillments(ctx context.Context, records ...*fulfillment.Record) error
	UpdateFulfillment(ctx context.Context, record *fulfillment.Record) error

	// Intents
	// --------------------------------------------------------------------------------
	SaveIntent(ctx context.Context, record *intent.Record) error
	GetIntent(ctx context.Context, intentID string) (*intent.Record, error)
	GetIntentBySignature(ctx context.Context, signature string) (*intent.Record, error)
	GetAllIntentsByOwner(ctx context.Context, owner string, opts ...query.Option) ([]*intent.Record, error)
	GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error)
	GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error)
	GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, owner string, since time.Time) (uint64, float64, error)

	// Merkle Trees
	// --------------------------------------------------------------------------------
	InitializeNewMerkleTree(ctx context.Context, name string, levels uint8, seeds []merkletree.Seed, readOnly bool) (*merkletree.MerkleTree, error)
	LoadExistingMerkleTree(ctx context.Context, name string, readOnly bool) (*merkletree.MerkleTree, error)

	// Messaging
	// --------------------------------------------------------------------------------
	CreateMessage(ctx context.Context, record *messaging.Record) error
	GetMessages(ctx context.Context, account string) ([]*messaging.Record, error)
	DeleteMessage(ctx context.Context, account string, messageID uuid.UUID) error

	// Nonces
	// --------------------------------------------------------------------------------
	GetNonce(ctx context.Context, address string) (*nonce.Record, error)
	GetNonceCount(ctx context.Context, env nonce.Environment, instance string) (uint64, error)
	GetNonceCountByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State) (uint64, error)
	GetNonceCountByStateAndPurpose(ctx context.Context, env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) (uint64, error)
	GetAllNonceByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State, opts ...query.Option) ([]*nonce.Record, error)
	BatchClaimAvailableNoncesByPurpose(ctx context.Context, env nonce.Environment, instance string, purpose nonce.Purpose, limit int, nodeID string, minExpireAt, maxExpireAt time.Time) ([]*nonce.Record, error)
	SaveNonce(ctx context.Context, record *nonce.Record) error

	// Rendezvous
	// --------------------------------------------------------------------------------
	PutRendezvous(ctx context.Context, record *rendezvous.Record) error
	ExtendRendezvousExpiry(ctx context.Context, key, address string, expiry time.Time) error
	DeleteRendezvous(ctx context.Context, key, address string) error
	GetRendezvous(ctx context.Context, key string) (*rendezvous.Record, error)

	// Swaps
	// --------------------------------------------------------------------------------
	SaveSwap(ctx context.Context, record *swap.Record) error
	GetSwapById(ctx context.Context, id string) (*swap.Record, error)
	GetAllSwapsByOwnerAndState(ctx context.Context, owner string, state swap.State) ([]*swap.Record, error)
	GetAllSwapsByState(ctx context.Context, state swap.State, opts ...query.Option) ([]*swap.Record, error)

	// Timelocks
	// --------------------------------------------------------------------------------
	SaveTimelock(ctx context.Context, record *timelock.Record) error
	GetTimelockByAddress(ctx context.Context, address string) (*timelock.Record, error)
	GetTimelockByVault(ctx context.Context, vault string) (*timelock.Record, error)
	GetTimelockByDepositPda(ctx context.Context, depositPda string) (*timelock.Record, error)
	GetTimelockBySwapPda(ctx context.Context, swapPda string) (*timelock.Record, error)
	GetTimelockByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error)
	GetAllTimelocksByState(ctx context.Context, state timelock_token.TimelockState, opts ...query.Option) ([]*timelock.Record, error)
	GetTimelockCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error)

	// Transactions
	// --------------------------------------------------------------------------------
	GetTransaction(ctx context.Context, sig string) (*transaction.Record, error)
	SaveTransaction(ctx context.Context, record *transaction.Record) error

	// Vault
	// --------------------------------------------------------------------------------
	GetKey(ctx context.Context, public_key string) (*vault.Record, error)
	GetKeyCount(ctx context.Context) (uint64, error)
	GetKeyCountByState(ctx context.Context, state vault.State) (uint64, error)
	GetAllKeysByState(ctx context.Context, state vault.State, opts ...query.Option) ([]*vault.Record, error)
	SaveKey(ctx context.Context, record *vault.Record) error

	// ExecuteInTx executes fn with a single DB transaction that is scoped to the call.
	// This enables more complex transactions that can span many calls across the provider.
	//
	// Note: This highly relies on the store implementations adding explicit support for
	// this, which was added way later than when most were written. When using this
	// function, ensure there is proper support for whatever is being called inside fn.
	ExecuteInTx(ctx context.Context, isolation sql.IsolationLevel, fn func(ctx context.Context) error) error
}

type DatabaseProvider struct {
	accounts     account.Store
	actions      action.Store
	balance      balance.Store
	currencies   currency.Store
	cvmRam       cvm_ram.Store
	cvmStorage   cvm_storage.Store
	deposits     deposit.Store
	fulfillments fulfillment.Store
	intents      intent.Store
	merkleTrees  merkletree.Store
	messages     messaging.Store
	nonces       nonce.Store
	rendezvous   rendezvous.Store
	swaps        swap.Store
	timelocks    timelock.Store
	transactions transaction.Store
	vault        vault.Store

	exchangeCache cache.Cache
	timelockCache cache.Cache

	db *sqlx.DB
}

func NewDatabaseProvider(dbConfig *pg.Config) (DatabaseData, error) {
	db, err := pg.NewWithUsernameAndPassword(
		dbConfig.User,
		dbConfig.Password,
		dbConfig.Host,
		fmt.Sprint(dbConfig.Port),
		dbConfig.DbName,
	)
	if err != nil {
		return nil, err
	}

	if dbConfig.MaxOpenConnections > 0 {
		db.SetMaxOpenConns(dbConfig.MaxOpenConnections)
	}
	if dbConfig.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(dbConfig.MaxIdleConnections)
	}
	db.SetConnMaxIdleTime(time.Hour)
	db.SetConnMaxLifetime(time.Hour)

	return &DatabaseProvider{
		accounts:     account_postgres_client.New(db),
		actions:      action_postgres_client.New(db),
		balance:      balance_postgres_client.New(db),
		currencies:   currency_postgres_client.New(db),
		cvmRam:       cvm_ram_postgres_client.New(db),
		cvmStorage:   cvm_storage_postgres_client.New(db),
		deposits:     deposit_postgres_client.New(db),
		fulfillments: fulfillment_postgres_client.New(db),
		intents:      intent_postgres_client.New(db),
		merkleTrees:  merkletree_postgres_client.New(db),
		messages:     messaging_postgres_client.New(db),
		nonces:       nonce_postgres_client.New(db),
		rendezvous:   rendezvous_postgres_client.New(db),
		swaps:        swap_postgres_client.New(db),
		timelocks:    timelock_postgres_client.New(db),
		transactions: transaction_postgres_client.New(db),
		vault:        vault_postgres_client.New(db),

		exchangeCache: cache.NewCache(maxExchangeRateCacheBudget),
		timelockCache: cache.NewCache(maxTimelockCacheBudget),

		db: sqlx.NewDb(db, "pgx"),
	}, nil
}

func NewTestDatabaseProvider() DatabaseData {
	return &DatabaseProvider{
		accounts:     account_memory_client.New(),
		actions:      action_memory_client.New(),
		balance:      balance_memory_client.New(),
		currencies:   currency_memory_client.New(),
		cvmRam:       cvm_ram_memory_client.New(),
		cvmStorage:   cvm_storage_memory_client.New(),
		deposits:     deposit_memory_client.New(),
		fulfillments: fulfillment_memory_client.New(),
		intents:      intent_memory_client.New(),
		merkleTrees:  merkletree_memory_client.New(),
		messages:     messaging_memory_client.New(),
		nonces:       nonce_memory_client.New(),
		rendezvous:   rendezvous_memory_client.New(),
		swaps:        swap_memory_client.New(),
		timelocks:    timelock_memory_client.New(),
		transactions: transaction_memory_client.New(),
		vault:        vault_memory_client.New(),

		exchangeCache: cache.NewCache(maxExchangeRateCacheBudget),
		timelockCache: nil, // Shouldn't be used for tests
	}
}

func (dp *DatabaseProvider) ExecuteInTx(ctx context.Context, isolation sql.IsolationLevel, fn func(ctx context.Context) error) error {
	if dp.db == nil {
		return fn(ctx)
	}

	return pg.ExecuteTxWithinCtx(ctx, dp.db, isolation, fn)
}

// Account Info
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) CreateAccountInfo(ctx context.Context, record *account.Record) error {
	return dp.accounts.Put(ctx, record)
}
func (dp *DatabaseProvider) UpdateAccountInfo(ctx context.Context, record *account.Record) error {
	return dp.accounts.Update(ctx, record)
}
func (dp *DatabaseProvider) GetAccountInfoByTokenAddress(ctx context.Context, address string) (*account.Record, error) {
	return dp.accounts.GetByTokenAddress(ctx, address)
}
func (dp *DatabaseProvider) GetAccountInfoByTokenAddressBatch(ctx context.Context, addresses ...string) (map[string]*account.Record, error) {
	return dp.accounts.GetByTokenAddressBatch(ctx, addresses...)
}
func (dp *DatabaseProvider) GetAccountInfoByAuthorityAddress(ctx context.Context, address string) (map[string]*account.Record, error) {
	return dp.accounts.GetByAuthorityAddress(ctx, address)
}
func (dp *DatabaseProvider) GetLatestAccountInfosByOwnerAddress(ctx context.Context, address string) (map[string]map[commonpb.AccountType][]*account.Record, error) {
	return dp.accounts.GetLatestByOwnerAddress(ctx, address)
}
func (dp *DatabaseProvider) GetLatestAccountInfoByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (map[string]*account.Record, error) {
	return dp.accounts.GetLatestByOwnerAddressAndType(ctx, address, accountType)
}
func (dp *DatabaseProvider) GetPrioritizedAccountInfosRequiringDepositSync(ctx context.Context, limit uint64) ([]*account.Record, error) {
	return dp.accounts.GetPrioritizedRequiringDepositSync(ctx, limit)
}
func (dp *DatabaseProvider) GetPrioritizedAccountInfosRequiringAutoReturnCheck(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error) {
	return dp.accounts.GetPrioritizedRequiringAutoReturnCheck(ctx, maxAge, limit)
}
func (dp *DatabaseProvider) GetAccountInfoCountRequiringDepositSync(ctx context.Context) (uint64, error) {
	return dp.accounts.CountRequiringDepositSync(ctx)
}
func (dp *DatabaseProvider) GetAccountInfoCountRequiringAutoReturnCheck(ctx context.Context) (uint64, error) {
	return dp.accounts.CountRequiringAutoReturnCheck(ctx)
}

// Actions
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutAllActions(ctx context.Context, records ...*action.Record) error {
	return dp.actions.PutAll(ctx, records...)
}
func (dp *DatabaseProvider) UpdateAction(ctx context.Context, record *action.Record) error {
	return dp.actions.Update(ctx, record)
}
func (dp *DatabaseProvider) GetActionById(ctx context.Context, intent string, actionId uint32) (*action.Record, error) {
	return dp.actions.GetById(ctx, intent, actionId)
}
func (dp *DatabaseProvider) GetAllActionsByIntent(ctx context.Context, intent string) ([]*action.Record, error) {
	return dp.actions.GetAllByIntent(ctx, intent)
}
func (dp *DatabaseProvider) GetAllActionsByAddress(ctx context.Context, address string) ([]*action.Record, error) {
	return dp.actions.GetAllByAddress(ctx, address)
}
func (dp *DatabaseProvider) GetNetBalanceFromActions(ctx context.Context, address string) (int64, error) {
	return dp.actions.GetNetBalance(ctx, address)
}
func (dp *DatabaseProvider) GetNetBalanceFromActionsBatch(ctx context.Context, accounts ...string) (map[string]int64, error) {
	return dp.actions.GetNetBalanceBatch(ctx, accounts...)
}
func (dp *DatabaseProvider) GetGiftCardClaimedAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	return dp.actions.GetGiftCardClaimedAction(ctx, giftCardVault)
}
func (dp *DatabaseProvider) GetGiftCardAutoReturnAction(ctx context.Context, giftCardVault string) (*action.Record, error) {
	return dp.actions.GetGiftCardAutoReturnAction(ctx, giftCardVault)
}
func (dp *DatabaseProvider) CountFeeActions(ctx context.Context, intent string, feeType transactionpb.FeePaymentAction_FeeType) (uint64, error) {
	return dp.actions.CountFeeActions(ctx, intent, feeType)
}
func (dp *DatabaseProvider) HasFeeAction(ctx context.Context, intent string, feeType transactionpb.FeePaymentAction_FeeType) (bool, error) {
	count, err := dp.actions.CountFeeActions(ctx, intent, feeType)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Balance
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetCachedBalanceVersion(ctx context.Context, account string) (uint64, error) {
	return dp.balance.GetCachedVersion(ctx, account)
}
func (dp *DatabaseProvider) AdvanceCachedBalanceVersion(ctx context.Context, account string, currentVersion uint64) error {
	return dp.balance.AdvanceCachedVersion(ctx, account, currentVersion)
}
func (dp *DatabaseProvider) CheckNotClosedForBalanceUpdate(ctx context.Context, account string) error {
	return dp.balance.CheckNotClosed(ctx, account)
}
func (dp *DatabaseProvider) MarkAsClosedForBalanceUpdate(ctx context.Context, account string) error {
	return dp.balance.MarkAsClosed(ctx, account)
}
func (dp *DatabaseProvider) SaveExternalBalanceCheckpoint(ctx context.Context, record *balance.ExternalCheckpointRecord) error {
	return dp.balance.SaveExternalCheckpoint(ctx, record)
}
func (dp *DatabaseProvider) GetExternalBalanceCheckpoint(ctx context.Context, account string) (*balance.ExternalCheckpointRecord, error) {
	return dp.balance.GetExternalCheckpoint(ctx, account)
}

// Currencies
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetExchangeRate(ctx context.Context, code currency_lib.Code, t time.Time) (*currency.ExchangeRateRecord, error) {
	key := fmt.Sprintf("%s:%s", code, t.Truncate(5*time.Minute).Format(time.RFC3339))
	if rate, ok := dp.exchangeCache.Retrieve(key); ok {
		return rate.(*currency.ExchangeRateRecord), nil
	}

	rate, err := dp.currencies.GetExchangeRate(ctx, string(code), t)
	if err != nil {
		return nil, err
	}

	dp.exchangeCache.Insert(key, rate, singleExchangeRateCacheWeight)

	return rate, nil
}
func (dp *DatabaseProvider) GetAllExchangeRates(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	key := fmt.Sprintf("everything:%s", t.Truncate(5*time.Minute).Format(time.RFC3339))
	if rates, ok := dp.exchangeCache.Retrieve(key); ok {
		return rates.(*currency.MultiRateRecord), nil
	}

	rates, err := dp.currencies.GetAllExchangeRates(ctx, t)
	if err != nil {
		return nil, err
	}
	dp.exchangeCache.Insert(key, rates, multiExchangeRateCacheWeight)

	return rates, nil
}
func (dp *DatabaseProvider) GetExchangeRateHistory(ctx context.Context, code currency_lib.Code, opts ...query.Option) ([]*currency.ExchangeRateRecord, error) {
	req := query.QueryOptions{
		Limit:     maxCurrencyHistoryReqSize,
		End:       time.Now(),
		SortBy:    query.Ascending,
		Supported: query.CanLimitResults | query.CanSortBy | query.CanBucketBy | query.CanQueryByStartTime | query.CanQueryByEndTime,
	}
	req.Apply(opts...)

	if req.Start.IsZero() {
		return nil, query.ErrQueryNotSupported
	}
	if req.Limit > maxCurrencyHistoryReqSize {
		return nil, query.ErrQueryNotSupported
	}

	return dp.currencies.GetExchangeRatesInRange(ctx, string(code), req.Interval, req.Start, req.End, req.SortBy)
}
func (dp *DatabaseProvider) ImportExchangeRates(ctx context.Context, data *currency.MultiRateRecord) error {
	return dp.currencies.PutExchangeRates(ctx, data)
}
func (dp *DatabaseProvider) PutCurrencyMetadata(ctx context.Context, record *currency.MetadataRecord) error {
	return dp.currencies.PutMetadata(ctx, record)
}
func (dp *DatabaseProvider) GetCurrencyMetadata(ctx context.Context, mint string) (*currency.MetadataRecord, error) {
	return dp.currencies.GetMetadata(ctx, mint)
}
func (dp *DatabaseProvider) PutCurrencyReserve(ctx context.Context, record *currency.ReserveRecord) error {
	return dp.currencies.PutReserveRecord(ctx, record)
}
func (dp *DatabaseProvider) GetCurrencyReserveAtTime(ctx context.Context, mint string, t time.Time) (*currency.ReserveRecord, error) {
	return dp.currencies.GetReserveAtTime(ctx, mint, t)
}

// CVM RAM
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) InitializeVmMemory(ctx context.Context, record *cvm_ram.Record) error {
	return dp.cvmRam.InitializeMemory(ctx, record)
}
func (dp *DatabaseProvider) FreeVmMemoryByIndex(ctx context.Context, memoryAccount string, index uint16) error {
	return dp.cvmRam.FreeMemoryByIndex(ctx, memoryAccount, index)
}
func (dp *DatabaseProvider) FreeVmMemoryByAddress(ctx context.Context, address string) error {
	return dp.cvmRam.FreeMemoryByAddress(ctx, address)
}
func (dp *DatabaseProvider) ReserveVmMemory(ctx context.Context, vm string, accountType cvm.VirtualAccountType, address string) (string, uint16, error) {
	return dp.cvmRam.ReserveMemory(ctx, vm, accountType, address)
}

// CVM Storage
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) InitializeVmStorage(ctx context.Context, record *cvm_storage.Record) error {
	return dp.cvmStorage.InitializeStorage(ctx, record)
}
func (dp *DatabaseProvider) FindAnyVmStorageWithAvailableCapacity(ctx context.Context, vm string, purpose cvm_storage.Purpose, minCapacity uint64) (*cvm_storage.Record, error) {
	return dp.cvmStorage.FindAnyWithAvailableCapacity(ctx, vm, purpose, minCapacity)
}
func (dp *DatabaseProvider) ReserveVmStorage(ctx context.Context, vm string, purpose cvm_storage.Purpose, address string) (string, error) {
	return dp.cvmStorage.ReserveStorage(ctx, vm, purpose, address)
}

// Deposits
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveExternalDeposit(ctx context.Context, record *deposit.Record) error {
	return dp.deposits.Save(ctx, record)
}
func (dp *DatabaseProvider) GetExternalDeposit(ctx context.Context, signature, account string) (*deposit.Record, error) {
	return dp.deposits.Get(ctx, signature, account)
}
func (dp *DatabaseProvider) GetTotalExternalDepositedAmountInQuarks(ctx context.Context, account string) (uint64, error) {
	return dp.deposits.GetQuarkAmount(ctx, account)
}
func (dp *DatabaseProvider) GetTotalExternalDepositedAmountInQuarksBatch(ctx context.Context, accounts ...string) (map[string]uint64, error) {
	return dp.deposits.GetQuarkAmountBatch(ctx, accounts...)
}
func (dp *DatabaseProvider) GetTotalExternalDepositedAmountInUsd(ctx context.Context, account string) (float64, error) {
	return dp.deposits.GetUsdAmount(ctx, account)
}

// Fulfillments
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetFulfillmentById(ctx context.Context, id uint64) (*fulfillment.Record, error) {
	return dp.fulfillments.GetById(ctx, id)
}
func (dp *DatabaseProvider) GetFulfillmentBySignature(ctx context.Context, signature string) (*fulfillment.Record, error) {
	return dp.fulfillments.GetBySignature(ctx, signature)
}
func (dp *DatabaseProvider) GetFulfillmentByVirtualSignature(ctx context.Context, signature string) (*fulfillment.Record, error) {
	return dp.fulfillments.GetByVirtualSignature(ctx, signature)
}
func (dp *DatabaseProvider) GetFulfillmentCount(ctx context.Context) (uint64, error) {
	return dp.fulfillments.Count(ctx)
}
func (dp *DatabaseProvider) GetFulfillmentCountByState(ctx context.Context, state fulfillment.State) (uint64, error) {
	return dp.fulfillments.CountByState(ctx, state)
}
func (dp *DatabaseProvider) GetFulfillmentCountByStateGroupedByType(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	return dp.fulfillments.CountByStateGroupedByType(ctx, state)
}
func (dp *DatabaseProvider) GetFulfillmentCountForMetrics(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	return dp.fulfillments.CountForMetrics(ctx, state)
}
func (dp *DatabaseProvider) GetFulfillmentCountByStateAndAddress(ctx context.Context, state fulfillment.State, address string) (uint64, error) {
	return dp.fulfillments.CountByStateAndAddress(ctx, state, address)
}
func (dp *DatabaseProvider) GetFulfillmentCountByTypeStateAndAddress(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	return dp.fulfillments.CountByTypeStateAndAddress(ctx, fulfillmentType, state, address)
}
func (dp *DatabaseProvider) GetFulfillmentCountByTypeStateAndAddressAsSource(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	return dp.fulfillments.CountByTypeStateAndAddressAsSource(ctx, fulfillmentType, state, address)
}
func (dp *DatabaseProvider) GetFulfillmentCountByIntentAndState(ctx context.Context, intent string, state fulfillment.State) (uint64, error) {
	return dp.fulfillments.CountByIntentAndState(ctx, intent, state)
}
func (dp *DatabaseProvider) GetFulfillmentCountByIntent(ctx context.Context, intent string) (uint64, error) {
	return dp.fulfillments.CountByIntent(ctx, intent)
}
func (dp *DatabaseProvider) GetFulfillmentCountByTypeActionAndState(ctx context.Context, intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) (uint64, error) {
	return dp.fulfillments.CountByTypeActionAndState(ctx, intentId, actionId, fulfillmentType, state)
}
func (dp *DatabaseProvider) GetPendingFulfillmentCountByType(ctx context.Context) (map[fulfillment.Type]uint64, error) {
	return dp.fulfillments.CountPendingByType(ctx)
}
func (dp *DatabaseProvider) GetAllFulfillmentsByState(ctx context.Context, state fulfillment.State, includeDisabledActiveScheduling bool, opts ...query.Option) ([]*fulfillment.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.fulfillments.GetAllByState(ctx, state, includeDisabledActiveScheduling, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetAllFulfillmentsByIntent(ctx context.Context, intent string, opts ...query.Option) ([]*fulfillment.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.fulfillments.GetAllByIntent(ctx, intent, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetAllFulfillmentsByAction(ctx context.Context, intentId string, actionId uint32) ([]*fulfillment.Record, error) {
	return dp.fulfillments.GetAllByAction(ctx, intentId, actionId)
}
func (dp *DatabaseProvider) GetAllFulfillmentsByTypeAndAction(ctx context.Context, fulfillmentType fulfillment.Type, intentId string, actionId uint32) ([]*fulfillment.Record, error) {
	return dp.fulfillments.GetAllByTypeAndAction(ctx, fulfillmentType, intentId, actionId)
}
func (dp *DatabaseProvider) GetFirstSchedulableFulfillmentByAddressAsSource(ctx context.Context, address string) (*fulfillment.Record, error) {
	return dp.fulfillments.GetFirstSchedulableByAddressAsSource(ctx, address)
}
func (dp *DatabaseProvider) GetFirstSchedulableFulfillmentByAddressAsDestination(ctx context.Context, address string) (*fulfillment.Record, error) {
	return dp.fulfillments.GetFirstSchedulableByAddressAsDestination(ctx, address)
}
func (dp *DatabaseProvider) GetFirstSchedulableFulfillmentByType(ctx context.Context, fulfillmentType fulfillment.Type) (*fulfillment.Record, error) {
	return dp.fulfillments.GetFirstSchedulableByType(ctx, fulfillmentType)
}
func (dp *DatabaseProvider) GetNextSchedulableFulfillmentByAddress(ctx context.Context, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*fulfillment.Record, error) {
	return dp.fulfillments.GetNextSchedulableByAddress(ctx, address, intentOrderingIndex, actionOrderingIndex, fulfillmentOrderingIndex)
}
func (dp *DatabaseProvider) PutAllFulfillments(ctx context.Context, records ...*fulfillment.Record) error {
	return dp.fulfillments.PutAll(ctx, records...)
}
func (dp *DatabaseProvider) UpdateFulfillment(ctx context.Context, record *fulfillment.Record) error {
	return dp.fulfillments.Update(ctx, record)
}

// Intents
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetIntent(ctx context.Context, intentID string) (*intent.Record, error) {
	return dp.intents.Get(ctx, intentID)
}
func (dp *DatabaseProvider) GetIntentBySignature(ctx context.Context, signature string) (*intent.Record, error) {
	fulfillmentRecord, err := dp.fulfillments.GetBySignature(ctx, signature)
	if err == fulfillment.ErrFulfillmentNotFound {
		return nil, intent.ErrIntentNotFound
	} else if err != nil {
		return nil, err
	}

	return dp.intents.Get(ctx, fulfillmentRecord.Intent)
}
func (dp *DatabaseProvider) GetAllIntentsByOwner(ctx context.Context, owner string, opts ...query.Option) ([]*intent.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.intents.GetAllByOwner(ctx, owner, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	return dp.intents.GetOriginalGiftCardIssuedIntent(ctx, giftCardVault)
}
func (dp *DatabaseProvider) GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	return dp.intents.GetGiftCardClaimedIntent(ctx, giftCardVault)
}
func (dp *DatabaseProvider) SaveIntent(ctx context.Context, record *intent.Record) error {
	return dp.intents.Save(ctx, record)
}
func (dp *DatabaseProvider) GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, owner string, since time.Time) (uint64, float64, error) {
	return dp.intents.GetTransactedAmountForAntiMoneyLaundering(ctx, owner, since)
}

// Merkle Trees
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) InitializeNewMerkleTree(ctx context.Context, name string, levels uint8, seeds []merkletree.Seed, readOnly bool) (*merkletree.MerkleTree, error) {
	return merkletree.InitializeNew(ctx, dp.merkleTrees, name, levels, seeds, readOnly)
}
func (dp *DatabaseProvider) LoadExistingMerkleTree(ctx context.Context, name string, readOnly bool) (*merkletree.MerkleTree, error) {
	return merkletree.LoadExisting(ctx, dp.merkleTrees, name, readOnly)
}

// Messaging
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) CreateMessage(ctx context.Context, record *messaging.Record) error {
	return dp.messages.Insert(ctx, record)
}
func (dp *DatabaseProvider) GetMessages(ctx context.Context, account string) ([]*messaging.Record, error) {
	return dp.messages.Get(ctx, account)
}
func (dp *DatabaseProvider) DeleteMessage(ctx context.Context, account string, messageID uuid.UUID) error {
	return dp.messages.Delete(ctx, account, messageID)
}

// Nonces
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetNonce(ctx context.Context, address string) (*nonce.Record, error) {
	return dp.nonces.Get(ctx, address)
}
func (dp *DatabaseProvider) GetNonceCount(ctx context.Context, env nonce.Environment, instance string) (uint64, error) {
	return dp.nonces.Count(ctx, env, instance)
}
func (dp *DatabaseProvider) GetNonceCountByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State) (uint64, error) {
	return dp.nonces.CountByState(ctx, env, instance, state)
}
func (dp *DatabaseProvider) GetNonceCountByStateAndPurpose(ctx context.Context, env nonce.Environment, instance string, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	return dp.nonces.CountByStateAndPurpose(ctx, env, instance, state, purpose)
}
func (dp *DatabaseProvider) GetAllNonceByState(ctx context.Context, env nonce.Environment, instance string, state nonce.State, opts ...query.Option) ([]*nonce.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.nonces.GetAllByState(ctx, env, instance, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) BatchClaimAvailableNoncesByPurpose(ctx context.Context, env nonce.Environment, instance string, purpose nonce.Purpose, limit int, nodeID string, minExpireAt, maxExpireAt time.Time) ([]*nonce.Record, error) {
	return dp.nonces.BatchClaimAvailableByPurpose(ctx, env, instance, purpose, limit, nodeID, minExpireAt, maxExpireAt)
}
func (dp *DatabaseProvider) SaveNonce(ctx context.Context, record *nonce.Record) error {
	return dp.nonces.Save(ctx, record)
}

// Rendezvous
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutRendezvous(ctx context.Context, record *rendezvous.Record) error {
	return dp.rendezvous.Put(ctx, record)
}
func (dp *DatabaseProvider) ExtendRendezvousExpiry(ctx context.Context, key, address string, expiry time.Time) error {
	return dp.rendezvous.ExtendExpiry(ctx, key, address, expiry)
}
func (dp *DatabaseProvider) DeleteRendezvous(ctx context.Context, key, address string) error {
	return dp.rendezvous.Delete(ctx, key, address)
}
func (dp *DatabaseProvider) GetRendezvous(ctx context.Context, key string) (*rendezvous.Record, error) {
	return dp.rendezvous.Get(ctx, key)
}

// Swaps
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveSwap(ctx context.Context, record *swap.Record) error {
	return dp.swaps.Save(ctx, record)
}
func (dp *DatabaseProvider) GetSwapById(ctx context.Context, id string) (*swap.Record, error) {
	return dp.swaps.GetById(ctx, id)
}
func (dp *DatabaseProvider) GetSwapByFundingId(ctx context.Context, fundingId string) (*swap.Record, error) {
	return dp.swaps.GetByFundingId(ctx, fundingId)
}
func (dp *DatabaseProvider) GetAllSwapsByOwnerAndState(ctx context.Context, owner string, state swap.State) ([]*swap.Record, error) {
	return dp.swaps.GetAllByOwnerAndState(ctx, owner, state)
}
func (dp *DatabaseProvider) GetAllSwapsByState(ctx context.Context, state swap.State, opts ...query.Option) ([]*swap.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}
	return dp.swaps.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}

// Timelocks
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveTimelock(ctx context.Context, record *timelock.Record) error {
	return dp.timelocks.Save(ctx, record)
}
func (dp *DatabaseProvider) GetTimelockByAddress(ctx context.Context, address string) (*timelock.Record, error) {
	// todo: add caching if this becomes a heavy hitter like GetByVault
	return dp.timelocks.GetByAddress(ctx, address)
}
func (dp *DatabaseProvider) GetTimelockByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error) {
	records, err := dp.timelocks.GetByVaultBatch(ctx, vaults...)
	if err != nil {
		return nil, err
	}

	// Don't use a cache if it hasn't been setup (eg. test implementation)
	if dp.timelockCache == nil {
		return records, nil
	}

	for _, record := range records {
		cached, ok := dp.timelockCache.Retrieve(record.VaultAddress)
		if ok {
			cacheEntry := cached.(*timelockCacheEntry)
			cacheEntry.mu.Lock()
			cacheEntry.record = record.Clone()
			cacheEntry.lastUpdatedAt = time.Now()
			cacheEntry.mu.Unlock()
		} else {
			cacheEntry := &timelockCacheEntry{
				record:        record.Clone(),
				lastUpdatedAt: time.Now(),
			}
			dp.timelockCache.Insert(record.VaultAddress, cacheEntry, 1)
		}
	}

	return records, nil
}
func (dp *DatabaseProvider) GetTimelockByVault(ctx context.Context, vault string) (*timelock.Record, error) {
	// Don't use a cache if it hasn't been setup (eg. test implementation)
	if dp.timelockCache == nil {
		return dp.timelocks.GetByVault(ctx, vault)
	}

	// todo: Use a cache implementation that has TTLs and refreshes lol
	cached, ok := dp.timelockCache.Retrieve(vault)
	if ok {
		// First do an optimized cache value check using a read lock
		cacheEntry := cached.(*timelockCacheEntry)
		cacheEntry.mu.RLock()
		if time.Since(cacheEntry.lastUpdatedAt) < timelockCacheTTL {
			cacheEntry.mu.RUnlock()
			return cacheEntry.record.Clone(), nil
		}
		cacheEntry.mu.RUnlock()

		// Cache value is stale, so acquire the write lock in an attempt
		// to refresh the value.
		cacheEntry.mu.Lock()
		defer cacheEntry.mu.Unlock()

		// Check the cache value state again in the event we lost the race to
		// updated the value
		if time.Since(cacheEntry.lastUpdatedAt) < timelockCacheTTL {
			return cacheEntry.record.Clone(), nil
		}

		// Cached value is still stale, so fetch from the DB
		record, err := dp.timelocks.GetByVault(ctx, vault)
		if err == nil {
			cacheEntry.record = record.Clone()
			cacheEntry.lastUpdatedAt = time.Now()
		}
		return record, err
	}

	// Record not cached, so fetch it and insert the initial cache entry
	record, err := dp.timelocks.GetByVault(ctx, vault)
	if err == nil {
		cacheEntry := &timelockCacheEntry{
			record:        record.Clone(),
			lastUpdatedAt: time.Now(),
		}
		dp.timelockCache.Insert(vault, cacheEntry, 1)
	}
	return record, err
}
func (dp *DatabaseProvider) GetTimelockByDepositPda(ctx context.Context, depositPda string) (*timelock.Record, error) {
	return dp.timelocks.GetByDepositPda(ctx, depositPda)
}
func (dp *DatabaseProvider) GetTimelockBySwapPda(ctx context.Context, swapPda string) (*timelock.Record, error) {
	return dp.timelocks.GetBySwapPda(ctx, swapPda)
}
func (dp *DatabaseProvider) GetAllTimelocksByState(ctx context.Context, state timelock_token.TimelockState, opts ...query.Option) ([]*timelock.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.timelocks.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetTimelockCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error) {
	return dp.timelocks.GetCountByState(ctx, state)
}

// Transactions
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetTransaction(ctx context.Context, sig string) (*transaction.Record, error) {
	return dp.transactions.Get(ctx, sig)
}
func (dp *DatabaseProvider) SaveTransaction(ctx context.Context, record *transaction.Record) error {
	return dp.transactions.Put(ctx, record)
}

// Vault
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetKey(ctx context.Context, public_key string) (*vault.Record, error) {
	return dp.vault.Get(ctx, public_key)
}
func (dp *DatabaseProvider) GetKeyCount(ctx context.Context) (uint64, error) {
	return dp.vault.Count(ctx)
}
func (dp *DatabaseProvider) GetKeyCountByState(ctx context.Context, state vault.State) (uint64, error) {
	return dp.vault.CountByState(ctx, state)
}
func (dp *DatabaseProvider) GetAllKeysByState(ctx context.Context, state vault.State, opts ...query.Option) ([]*vault.Record, error) {
	req, err := query.DefaultPaginationHandlerWithLimit(25, opts...)
	if err != nil {
		return nil, err
	}

	return dp.vault.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) SaveKey(ctx context.Context, record *vault.Record) error {
	return dp.vault.Save(ctx, record)
}
