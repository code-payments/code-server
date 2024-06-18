package data

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"golang.org/x/text/language"

	"github.com/code-payments/code-server/pkg/cache"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	pg "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/database/query"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/airdrop"
	"github.com/code-payments/code-server/pkg/code/data/badgecount"
	"github.com/code-payments/code-server/pkg/code/data/balance"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	chat_v2 "github.com/code-payments/code-server/pkg/code/data/chat/v2"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/contact"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/event"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/login"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/onramp"
	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/paywall"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/code/data/user"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/code/data/user/storage"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/code/data/webhook"

	account_memory_client "github.com/code-payments/code-server/pkg/code/data/account/memory"
	action_memory_client "github.com/code-payments/code-server/pkg/code/data/action/memory"
	airdrop_memory_client "github.com/code-payments/code-server/pkg/code/data/airdrop/memory"
	badgecount_memory_client "github.com/code-payments/code-server/pkg/code/data/badgecount/memory"
	balance_memory_client "github.com/code-payments/code-server/pkg/code/data/balance/memory"
	chat_v1_memory_client "github.com/code-payments/code-server/pkg/code/data/chat/v1/memory"
	chat_v2_memory_client "github.com/code-payments/code-server/pkg/code/data/chat/v2/memory"
	commitment_memory_client "github.com/code-payments/code-server/pkg/code/data/commitment/memory"
	contact_memory_client "github.com/code-payments/code-server/pkg/code/data/contact/memory"
	currency_memory_client "github.com/code-payments/code-server/pkg/code/data/currency/memory"
	deposit_memory_client "github.com/code-payments/code-server/pkg/code/data/deposit/memory"
	event_memory_client "github.com/code-payments/code-server/pkg/code/data/event/memory"
	fulfillment_memory_client "github.com/code-payments/code-server/pkg/code/data/fulfillment/memory"
	intent_memory_client "github.com/code-payments/code-server/pkg/code/data/intent/memory"
	login_memory_client "github.com/code-payments/code-server/pkg/code/data/login/memory"
	merkletree_memory_client "github.com/code-payments/code-server/pkg/code/data/merkletree/memory"
	messaging "github.com/code-payments/code-server/pkg/code/data/messaging"
	messaging_memory_client "github.com/code-payments/code-server/pkg/code/data/messaging/memory"
	nonce_memory_client "github.com/code-payments/code-server/pkg/code/data/nonce/memory"
	onramp_memory_client "github.com/code-payments/code-server/pkg/code/data/onramp/memory"
	payment_memory_client "github.com/code-payments/code-server/pkg/code/data/payment/memory"
	paymentrequest_memory_client "github.com/code-payments/code-server/pkg/code/data/paymentrequest/memory"
	paywall_memory_client "github.com/code-payments/code-server/pkg/code/data/paywall/memory"
	phone_memory_client "github.com/code-payments/code-server/pkg/code/data/phone/memory"
	preferences_memory_client "github.com/code-payments/code-server/pkg/code/data/preferences/memory"
	push_memory_client "github.com/code-payments/code-server/pkg/code/data/push/memory"
	rendezvous_memory_client "github.com/code-payments/code-server/pkg/code/data/rendezvous/memory"
	timelock_memory_client "github.com/code-payments/code-server/pkg/code/data/timelock/memory"
	transaction_memory_client "github.com/code-payments/code-server/pkg/code/data/transaction/memory"
	treasury_memory_client "github.com/code-payments/code-server/pkg/code/data/treasury/memory"
	twitter_memory_client "github.com/code-payments/code-server/pkg/code/data/twitter/memory"
	user_identity_memory_client "github.com/code-payments/code-server/pkg/code/data/user/identity/memory"
	user_storage_memory_client "github.com/code-payments/code-server/pkg/code/data/user/storage/memory"
	vault_memory_client "github.com/code-payments/code-server/pkg/code/data/vault/memory"
	webhook_memory_client "github.com/code-payments/code-server/pkg/code/data/webhook/memory"

	account_postgres_client "github.com/code-payments/code-server/pkg/code/data/account/postgres"
	action_postgres_client "github.com/code-payments/code-server/pkg/code/data/action/postgres"
	airdrop_postgres_client "github.com/code-payments/code-server/pkg/code/data/airdrop/postgres"
	badgecount_postgres_client "github.com/code-payments/code-server/pkg/code/data/badgecount/postgres"
	balance_postgres_client "github.com/code-payments/code-server/pkg/code/data/balance/postgres"
	chat_v1_postgres_client "github.com/code-payments/code-server/pkg/code/data/chat/v1/postgres"
	commitment_postgres_client "github.com/code-payments/code-server/pkg/code/data/commitment/postgres"
	contact_postgres_client "github.com/code-payments/code-server/pkg/code/data/contact/postgres"
	currency_postgres_client "github.com/code-payments/code-server/pkg/code/data/currency/postgres"
	deposit_postgres_client "github.com/code-payments/code-server/pkg/code/data/deposit/postgres"
	event_postgres_client "github.com/code-payments/code-server/pkg/code/data/event/postgres"
	fulfillment_postgres_client "github.com/code-payments/code-server/pkg/code/data/fulfillment/postgres"
	intent_postgres_client "github.com/code-payments/code-server/pkg/code/data/intent/postgres"
	login_postgres_client "github.com/code-payments/code-server/pkg/code/data/login/postgres"
	merkletree_postgres_client "github.com/code-payments/code-server/pkg/code/data/merkletree/postgres"
	messaging_postgres_client "github.com/code-payments/code-server/pkg/code/data/messaging/postgres"
	nonce_postgres_client "github.com/code-payments/code-server/pkg/code/data/nonce/postgres"
	onramp_postgres_client "github.com/code-payments/code-server/pkg/code/data/onramp/postgres"
	payment_postgres_client "github.com/code-payments/code-server/pkg/code/data/payment/postgres"
	paymentrequest_postgres_client "github.com/code-payments/code-server/pkg/code/data/paymentrequest/postgres"
	paywall_postgres_client "github.com/code-payments/code-server/pkg/code/data/paywall/postgres"
	phone_postgres_client "github.com/code-payments/code-server/pkg/code/data/phone/postgres"
	preferences_postgres_client "github.com/code-payments/code-server/pkg/code/data/preferences/postgres"
	push_postgres_client "github.com/code-payments/code-server/pkg/code/data/push/postgres"
	rendezvous_postgres_client "github.com/code-payments/code-server/pkg/code/data/rendezvous/postgres"
	timelock_postgres_client "github.com/code-payments/code-server/pkg/code/data/timelock/postgres"
	transaction_postgres_client "github.com/code-payments/code-server/pkg/code/data/transaction/postgres"
	treasury_postgres_client "github.com/code-payments/code-server/pkg/code/data/treasury/postgres"
	twitter_postgres_client "github.com/code-payments/code-server/pkg/code/data/twitter/postgres"
	user_identity_postgres_client "github.com/code-payments/code-server/pkg/code/data/user/identity/postgres"
	user_storage_postgres_client "github.com/code-payments/code-server/pkg/code/data/user/storage/postgres"
	vault_postgres_client "github.com/code-payments/code-server/pkg/code/data/vault/postgres"
	webhook_postgres_client "github.com/code-payments/code-server/pkg/code/data/webhook/postgres"
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
	GetAccountInfoByAuthorityAddress(ctx context.Context, address string) (*account.Record, error)
	GetLatestAccountInfosByOwnerAddress(ctx context.Context, address string) (map[commonpb.AccountType][]*account.Record, error)
	GetLatestAccountInfoByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (*account.Record, error)
	GetRelationshipAccountInfoByOwnerAddress(ctx context.Context, address, relationshipTo string) (*account.Record, error)
	GetPrioritizedAccountInfosRequiringDepositSync(ctx context.Context, limit uint64) ([]*account.Record, error)
	GetPrioritizedAccountInfosRequiringAutoReturnCheck(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error)
	GetPrioritizedAccountInfosRequiringSwapRetry(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error)
	GetAccountInfoCountRequiringDepositSync(ctx context.Context) (uint64, error)
	GetAccountInfoCountRequiringAutoReturnCheck(ctx context.Context) (uint64, error)
	GetAccountInfoCountRequiringSwapRetry(ctx context.Context) (uint64, error)

	// Currency
	// --------------------------------------------------------------------------------
	GetExchangeRate(ctx context.Context, code currency_lib.Code, t time.Time) (*currency.ExchangeRateRecord, error)
	GetAllExchangeRates(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error)
	GetExchangeRateHistory(ctx context.Context, code currency_lib.Code, opts ...query.Option) ([]*currency.ExchangeRateRecord, error)
	ImportExchangeRates(ctx context.Context, data *currency.MultiRateRecord) error

	// Vault
	// --------------------------------------------------------------------------------
	GetKey(ctx context.Context, public_key string) (*vault.Record, error)
	GetKeyCount(ctx context.Context) (uint64, error)
	GetKeyCountByState(ctx context.Context, state vault.State) (uint64, error)
	GetAllKeysByState(ctx context.Context, state vault.State, opts ...query.Option) ([]*vault.Record, error)
	SaveKey(ctx context.Context, record *vault.Record) error

	// Nonce
	// --------------------------------------------------------------------------------
	GetNonce(ctx context.Context, address string) (*nonce.Record, error)
	GetNonceCount(ctx context.Context) (uint64, error)
	GetNonceCountByState(ctx context.Context, state nonce.State) (uint64, error)
	GetNonceCountByStateAndPurpose(ctx context.Context, state nonce.State, purpose nonce.Purpose) (uint64, error)
	GetAllNonceByState(ctx context.Context, state nonce.State, opts ...query.Option) ([]*nonce.Record, error)
	GetRandomAvailableNonceByPurpose(ctx context.Context, purpose nonce.Purpose) (*nonce.Record, error)
	SaveNonce(ctx context.Context, record *nonce.Record) error

	// Fulfillment
	// --------------------------------------------------------------------------------
	GetFulfillmentById(ctx context.Context, id uint64) (*fulfillment.Record, error)
	GetFulfillmentBySignature(ctx context.Context, signature string) (*fulfillment.Record, error)
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
	MarkFulfillmentAsActivelyScheduled(ctx context.Context, id uint64) error
	ActivelyScheduleTreasuryAdvanceFulfillments(ctx context.Context, treasury string, intentOrderingIndex uint64, limit int) (uint64, error)

	// Intent
	// --------------------------------------------------------------------------------
	SaveIntent(ctx context.Context, record *intent.Record) error
	GetIntent(ctx context.Context, intentID string) (*intent.Record, error)
	GetIntentBySignature(ctx context.Context, signature string) (*intent.Record, error)
	GetLatestIntentByInitiatorAndType(ctx context.Context, intentType intent.Type, owner string) (*intent.Record, error)
	GetAllIntentsByOwner(ctx context.Context, owner string, opts ...query.Option) ([]*intent.Record, error)
	GetIntentCountForAntispam(ctx context.Context, intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) (uint64, error)
	GetIntentCountWithOwnerInteractionsForAntispam(ctx context.Context, sourceOwner, destinationOwner string, states []intent.State, since time.Time) (uint64, error)
	GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error)
	GetDepositedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error)
	GetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, account string) (int64, error)
	GetLatestSaveRecentRootIntentForTreasury(ctx context.Context, treasury string) (*intent.Record, error)
	GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error)
	GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error)

	// Action
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

	// Payment (mostly deprecated for legacy accounts and unmigrated processes)
	// --------------------------------------------------------------------------------
	GetPayment(ctx context.Context, sig string, index int) (*payment.Record, error)
	CreatePayment(ctx context.Context, record *payment.Record) error
	UpdateOrCreatePayment(ctx context.Context, record *payment.Record) error
	GetPaymentHistory(ctx context.Context, account string, opts ...query.Option) ([]*payment.Record, error)
	GetPaymentHistoryWithinBlockRange(ctx context.Context, account string, lowerBound, upperBound uint64, opts ...query.Option) ([]*payment.Record, error)
	GetLegacyTotalExternalDepositAmountFromPrePrivacy2022Accounts(ctx context.Context, account string) (uint64, error)

	// Transaction
	// --------------------------------------------------------------------------------
	GetTransaction(ctx context.Context, sig string) (*transaction.Record, error)
	SaveTransaction(ctx context.Context, record *transaction.Record) error

	// Messaging
	// --------------------------------------------------------------------------------
	CreateMessage(ctx context.Context, record *messaging.Record) error
	GetMessages(ctx context.Context, account string) ([]*messaging.Record, error)
	DeleteMessage(ctx context.Context, account string, messageID uuid.UUID) error

	// Phone
	// --------------------------------------------------------------------------------
	SavePhoneVerification(ctx context.Context, v *phone.Verification) error
	GetPhoneVerification(ctx context.Context, account, phoneNumber string) (*phone.Verification, error)
	GetLatestPhoneVerificationForAccount(ctx context.Context, account string) (*phone.Verification, error)
	GetLatestPhoneVerificationForNumber(ctx context.Context, phoneNumber string) (*phone.Verification, error)
	GetAllPhoneVerificationsForNumber(ctx context.Context, phoneNumber string) ([]*phone.Verification, error)
	SavePhoneLinkingToken(ctx context.Context, token *phone.LinkingToken) error
	UsePhoneLinkingToken(ctx context.Context, phoneNumber, code string) error
	FilterVerifiedPhoneNumbers(ctx context.Context, phoneNumbers []string) ([]string, error)
	SaveOwnerAccountPhoneSetting(ctx context.Context, phoneNumber string, newSettings *phone.OwnerAccountSetting) error
	IsPhoneNumberLinkedToAccount(ctx context.Context, phoneNumber string, tokenAccount string) (bool, error)
	IsPhoneNumberEnabledForRemoteSendToAccount(ctx context.Context, phoneNumber string, tokenAccount string) (bool, error)
	PutPhoneEvent(ctx context.Context, event *phone.Event) error
	GetLatestPhoneEventForNumberByType(ctx context.Context, phoneNumber string, eventType phone.EventType) (*phone.Event, error)
	GetPhoneEventCountForVerificationByType(ctx context.Context, verification string, eventType phone.EventType) (uint64, error)
	GetPhoneEventCountForNumberByTypeSinceTimestamp(ctx context.Context, phoneNumber string, eventType phone.EventType, since time.Time) (uint64, error)
	GetUniquePhoneVerificationIdCountForNumberSinceTimestamp(ctx context.Context, phoneNumber string, since time.Time) (uint64, error)

	// Contact
	// --------------------------------------------------------------------------------
	AddContact(ctx context.Context, owner *user.DataContainerID, contact string) error
	BatchAddContacts(ctx context.Context, owner *user.DataContainerID, contacts []string) error
	RemoveContact(ctx context.Context, owner *user.DataContainerID, contact string) error
	BatchRemoveContacts(ctx context.Context, owner *user.DataContainerID, contacts []string) error
	GetContacts(ctx context.Context, owner *user.DataContainerID, limit uint32, pageToken []byte) (contacts []string, nextPageToken []byte, err error)

	// User Identity
	// --------------------------------------------------------------------------------
	PutUser(ctx context.Context, record *identity.Record) error
	GetUserByID(ctx context.Context, id *user.UserID) (*identity.Record, error)
	GetUserByPhoneView(ctx context.Context, phoneNumber string) (*identity.Record, error)

	// User Storage Management
	// --------------------------------------------------------------------------------
	PutUserDataContainer(ctx context.Context, container *storage.Record) error
	GetUserDataContainerByID(ctx context.Context, id *user.DataContainerID) (*storage.Record, error)
	GetUserDataContainerByPhone(ctx context.Context, tokenAccount, phoneNumber string) (*storage.Record, error)

	// Timelock
	// --------------------------------------------------------------------------------
	SaveTimelock(ctx context.Context, record *timelock.Record) error
	GetTimelockByAddress(ctx context.Context, address string) (*timelock.Record, error)
	GetTimelockByVault(ctx context.Context, vault string) (*timelock.Record, error)
	GetTimelockByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error)
	GetAllTimelocksByState(ctx context.Context, state timelock_token.TimelockState, opts ...query.Option) ([]*timelock.Record, error)
	GetTimelockCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error)

	// Push
	// --------------------------------------------------------------------------------
	PutPushToken(ctx context.Context, record *push.Record) error
	MarkPushTokenAsInvalid(ctx context.Context, pushToken string) error
	DeletePushToken(ctx context.Context, pushToken string) error
	GetAllValidPushTokensdByDataContainer(ctx context.Context, id *user.DataContainerID) ([]*push.Record, error)

	// Commitment
	// --------------------------------------------------------------------------------
	SaveCommitment(ctx context.Context, record *commitment.Record) error
	GetCommitmentByAddress(ctx context.Context, address string) (*commitment.Record, error)
	GetCommitmentByVault(ctx context.Context, vault string) (*commitment.Record, error)
	GetCommitmentByAction(ctx context.Context, intentId string, actionId uint32) (*commitment.Record, error)
	GetAllCommitmentsByState(ctx context.Context, state commitment.State, opts ...query.Option) ([]*commitment.Record, error)
	GetUpgradeableCommitmentsByOwner(ctx context.Context, owner string, limit uint64) ([]*commitment.Record, error)
	GetUsedTreasuryPoolDeficitFromCommitments(ctx context.Context, treasuryPool string) (uint64, error)
	GetTotalTreasuryPoolDeficitFromCommitments(ctx context.Context, treasuryPool string) (uint64, error)
	CountCommitmentsByState(ctx context.Context, state commitment.State) (uint64, error)
	CountCommitmentRepaymentsDivertedToVault(ctx context.Context, vault string) (uint64, error)

	// Treasury Pool
	// --------------------------------------------------------------------------------
	SaveTreasuryPool(ctx context.Context, record *treasury.Record) error
	GetTreasuryPoolByName(ctx context.Context, name string) (*treasury.Record, error)
	GetTreasuryPoolByAddress(ctx context.Context, address string) (*treasury.Record, error)
	GetTreasuryPoolByVault(ctx context.Context, vault string) (*treasury.Record, error)
	GetAllTreasuryPoolsByState(ctx context.Context, state treasury.TreasuryPoolState, opts ...query.Option) ([]*treasury.Record, error)
	SaveTreasuryPoolFunding(ctx context.Context, record *treasury.FundingHistoryRecord) error
	GetTotalAvailableTreasuryPoolFunds(ctx context.Context, vault string) (uint64, error)

	// Merkle Tree
	// --------------------------------------------------------------------------------
	InitializeNewMerkleTree(ctx context.Context, name string, levels uint8, seeds []merkletree.Seed, readOnly bool) (*merkletree.MerkleTree, error)
	LoadExistingMerkleTree(ctx context.Context, name string, readOnly bool) (*merkletree.MerkleTree, error)

	// External Deposits
	// --------------------------------------------------------------------------------
	SaveExternalDeposit(ctx context.Context, record *deposit.Record) error
	GetExternalDeposit(ctx context.Context, signature, destination string) (*deposit.Record, error)
	GetTotalExternalDepositedAmountInQuarks(ctx context.Context, account string) (uint64, error)
	GetTotalExternalDepositedAmountInQuarksBatch(ctx context.Context, accounts ...string) (map[string]uint64, error)
	GetTotalExternalDepositedAmountInUsd(ctx context.Context, account string) (float64, error)

	// Rendezvous
	// --------------------------------------------------------------------------------
	SaveRendezvous(ctx context.Context, record *rendezvous.Record) error
	GetRendezvous(ctx context.Context, key string) (*rendezvous.Record, error)
	DeleteRendezvous(ctx context.Context, key string) error

	// Requests
	// --------------------------------------------------------------------------------
	CreateRequest(ctx context.Context, record *paymentrequest.Record) error
	GetRequest(ctx context.Context, intentId string) (*paymentrequest.Record, error)

	// Paywall
	// --------------------------------------------------------------------------------
	CreatePaywall(ctx context.Context, record *paywall.Record) error
	GetPaywallByShortPath(ctx context.Context, path string) (*paywall.Record, error)

	// Event
	// --------------------------------------------------------------------------------
	SaveEvent(ctx context.Context, record *event.Record) error
	GetEvent(ctx context.Context, id string) (*event.Record, error)

	// Webhook
	// --------------------------------------------------------------------------------
	CreateWebhook(ctx context.Context, record *webhook.Record) error
	UpdateWebhook(ctx context.Context, record *webhook.Record) error
	GetWebhook(ctx context.Context, webhookId string) (*webhook.Record, error)
	CountWebhookByState(ctx context.Context, state webhook.State) (uint64, error)
	GetAllPendingWebhooksReadyToSend(ctx context.Context, limit uint64) ([]*webhook.Record, error)

	// Chat V1
	// --------------------------------------------------------------------------------
	PutChatV1(ctx context.Context, record *chat_v1.Chat) error
	GetChatByIdV1(ctx context.Context, chatId chat_v1.ChatId) (*chat_v1.Chat, error)
	GetAllChatsForUserV1(ctx context.Context, user string, opts ...query.Option) ([]*chat_v1.Chat, error)
	PutChatMessageV1(ctx context.Context, record *chat_v1.Message) error
	DeleteChatMessageV1(ctx context.Context, chatId chat_v1.ChatId, messageId string) error
	GetChatMessageV1(ctx context.Context, chatId chat_v1.ChatId, messageId string) (*chat_v1.Message, error)
	GetAllChatMessagesV1(ctx context.Context, chatId chat_v1.ChatId, opts ...query.Option) ([]*chat_v1.Message, error)
	AdvanceChatPointerV1(ctx context.Context, chatId chat_v1.ChatId, pointer string) error
	GetChatUnreadCountV1(ctx context.Context, chatId chat_v1.ChatId) (uint32, error)
	SetChatMuteStateV1(ctx context.Context, chatId chat_v1.ChatId, isMuted bool) error
	SetChatSubscriptionStateV1(ctx context.Context, chatId chat_v1.ChatId, isSubscribed bool) error

	// Chat V2
	// --------------------------------------------------------------------------------
	GetChatByIdV2(ctx context.Context, chatId chat_v2.ChatId) (*chat_v2.ChatRecord, error)
	GetChatMemberByIdV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId) (*chat_v2.MemberRecord, error)
	GetChatMessageByIdV2(ctx context.Context, chatId chat_v2.ChatId, messageId chat_v2.MessageId) (*chat_v2.MessageRecord, error)
	GetAllChatMembersV2(ctx context.Context, chatId chat_v2.ChatId) ([]*chat_v2.MemberRecord, error)
	GetPlatformUserChatMembershipV2(ctx context.Context, idByPlatform map[chat_v2.Platform]string, opts ...query.Option) ([]*chat_v2.MemberRecord, error)
	GetAllChatMessagesV2(ctx context.Context, chatId chat_v2.ChatId, opts ...query.Option) ([]*chat_v2.MessageRecord, error)
	GetChatUnreadCountV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, readPointer chat_v2.MessageId) (uint32, error)
	PutChatV2(ctx context.Context, record *chat_v2.ChatRecord) error
	PutChatMemberV2(ctx context.Context, record *chat_v2.MemberRecord) error
	PutChatMessageV2(ctx context.Context, record *chat_v2.MessageRecord) error
	AdvanceChatPointerV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, pointerType chat_v2.PointerType, pointer chat_v2.MessageId) (bool, error)
	UpgradeChatMemberIdentityV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, platform chat_v2.Platform, platformId string) error
	SetChatMuteStateV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, isMuted bool) error
	SetChatSubscriptionStateV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, isSubscribed bool) error

	// Badge Count
	// --------------------------------------------------------------------------------
	AddToBadgeCount(ctx context.Context, owner string, amount uint32) error
	ResetBadgeCount(ctx context.Context, owner string) error
	GetBadgeCount(ctx context.Context, owner string) (*badgecount.Record, error)

	// Login
	// --------------------------------------------------------------------------------
	SaveLogins(ctx context.Context, record *login.MultiRecord) error
	GetLoginsByAppInstall(ctx context.Context, appInstallId string) (*login.MultiRecord, error)
	GetLatestLoginByOwner(ctx context.Context, owner string) (*login.Record, error)

	// Balance
	// --------------------------------------------------------------------------------
	SaveBalanceCheckpoint(ctx context.Context, record *balance.Record) error
	GetBalanceCheckpoint(ctx context.Context, account string) (*balance.Record, error)

	// Onramp
	// --------------------------------------------------------------------------------
	PutFiatOnrampPurchase(ctx context.Context, record *onramp.Record) error
	GetFiatOnrampPurchase(ctx context.Context, nonce uuid.UUID) (*onramp.Record, error)

	// User Preferences
	// --------------------------------------------------------------------------------
	SaveUserPreferences(ctx context.Context, record *preferences.Record) error
	GetUserPreferences(ctx context.Context, id *user.DataContainerID) (*preferences.Record, error)
	GetUserLocale(ctx context.Context, owner string) (language.Tag, error)

	// Airdrop
	// --------------------------------------------------------------------------------
	MarkIneligibleForAirdrop(ctx context.Context, owner string) error
	IsEligibleForAirdrop(ctx context.Context, owner string) (bool, error)

	// Twitter
	// --------------------------------------------------------------------------------
	SaveTwitterUser(ctx context.Context, record *twitter.Record) error
	GetTwitterUserByUsername(ctx context.Context, username string) (*twitter.Record, error)
	GetTwitterUserByTipAddress(ctx context.Context, tipAddress string) (*twitter.Record, error)
	GetStaleTwitterUsers(ctx context.Context, minAge time.Duration, limit int) ([]*twitter.Record, error)
	MarkTweetAsProcessed(ctx context.Context, tweetId string) error
	IsTweetProcessed(ctx context.Context, tweetId string) (bool, error)
	MarkTwitterNonceAsUsed(ctx context.Context, tweetId string, nonce uuid.UUID) error

	// ExecuteInTx executes fn with a single DB transaction that is scoped to the call.
	// This enables more complex transactions that can span many calls across the provider.
	//
	// Note: This highly relies on the store implementations adding explicit support for
	// this, which was added way later than when most were written. When using this
	// function, ensure there is proper support for whatever is being called inside fn.
	ExecuteInTx(ctx context.Context, isolation sql.IsolationLevel, fn func(ctx context.Context) error) error
}

type DatabaseProvider struct {
	accounts       account.Store
	currencies     currency.Store
	vault          vault.Store
	nonces         nonce.Store
	fulfillments   fulfillment.Store
	intents        intent.Store
	actions        action.Store
	payments       payment.Store
	transactions   transaction.Store
	messages       messaging.Store
	phone          phone.Store
	contact        contact.Store
	userIdentity   identity.Store
	userStorage    storage.Store
	timelock       timelock.Store
	push           push.Store
	commitment     commitment.Store
	treasury       treasury.Store
	merkleTree     merkletree.Store
	deposits       deposit.Store
	rendezvous     rendezvous.Store
	paymentRequest paymentrequest.Store
	paywall        paywall.Store
	event          event.Store
	webhook        webhook.Store
	chatv1         chat_v1.Store
	chatv2         chat_v2.Store
	badgecount     badgecount.Store
	login          login.Store
	balance        balance.Store
	onramp         onramp.Store
	preferences    preferences.Store
	airdrop        airdrop.Store
	twitter        twitter.Store

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
		accounts:       account_postgres_client.New(db),
		currencies:     currency_postgres_client.New(db),
		nonces:         nonce_postgres_client.New(db),
		fulfillments:   fulfillment_postgres_client.New(db),
		intents:        intent_postgres_client.New(db),
		actions:        action_postgres_client.New(db),
		payments:       payment_postgres_client.New(db),
		transactions:   transaction_postgres_client.New(db),
		messages:       messaging_postgres_client.New(db),
		phone:          phone_postgres_client.New(db),
		contact:        contact_postgres_client.New(db),
		userIdentity:   user_identity_postgres_client.New(db),
		userStorage:    user_storage_postgres_client.New(db),
		timelock:       timelock_postgres_client.New(db),
		vault:          vault_postgres_client.New(db),
		push:           push_postgres_client.New(db),
		commitment:     commitment_postgres_client.New(db),
		treasury:       treasury_postgres_client.New(db),
		merkleTree:     merkletree_postgres_client.New(db),
		deposits:       deposit_postgres_client.New(db),
		rendezvous:     rendezvous_postgres_client.New(db),
		paymentRequest: paymentrequest_postgres_client.New(db),
		paywall:        paywall_postgres_client.New(db),
		event:          event_postgres_client.New(db),
		webhook:        webhook_postgres_client.New(db),
		chatv1:         chat_v1_postgres_client.New(db),
		chatv2:         chat_v2_memory_client.New(), // todo: Postgres version for production after PoC
		badgecount:     badgecount_postgres_client.New(db),
		login:          login_postgres_client.New(db),
		balance:        balance_postgres_client.New(db),
		onramp:         onramp_postgres_client.New(db),
		preferences:    preferences_postgres_client.New(db),
		airdrop:        airdrop_postgres_client.New(db),
		twitter:        twitter_postgres_client.New(db),

		exchangeCache: cache.NewCache(maxExchangeRateCacheBudget),
		timelockCache: cache.NewCache(maxTimelockCacheBudget),

		db: sqlx.NewDb(db, "pgx"),
	}, nil
}

func NewTestDatabaseProvider() DatabaseData {
	return &DatabaseProvider{
		accounts:       account_memory_client.New(),
		currencies:     currency_memory_client.New(),
		nonces:         nonce_memory_client.New(),
		fulfillments:   fulfillment_memory_client.New(),
		intents:        intent_memory_client.New(),
		actions:        action_memory_client.New(),
		payments:       payment_memory_client.New(),
		transactions:   transaction_memory_client.New(),
		phone:          phone_memory_client.New(),
		contact:        contact_memory_client.New(),
		userIdentity:   user_identity_memory_client.New(),
		userStorage:    user_storage_memory_client.New(),
		timelock:       timelock_memory_client.New(),
		vault:          vault_memory_client.New(),
		push:           push_memory_client.New(),
		commitment:     commitment_memory_client.New(),
		treasury:       treasury_memory_client.New(),
		merkleTree:     merkletree_memory_client.New(),
		messages:       messaging_memory_client.New(),
		deposits:       deposit_memory_client.New(),
		rendezvous:     rendezvous_memory_client.New(),
		paymentRequest: paymentrequest_memory_client.New(),
		paywall:        paywall_memory_client.New(),
		event:          event_memory_client.New(),
		webhook:        webhook_memory_client.New(),
		chatv1:         chat_v1_memory_client.New(),
		chatv2:         chat_v2_memory_client.New(),
		badgecount:     badgecount_memory_client.New(),
		login:          login_memory_client.New(),
		balance:        balance_memory_client.New(),
		onramp:         onramp_memory_client.New(),
		preferences:    preferences_memory_client.New(),
		airdrop:        airdrop_memory_client.New(),
		twitter:        twitter_memory_client.New(),

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
func (dp *DatabaseProvider) GetAccountInfoByAuthorityAddress(ctx context.Context, address string) (*account.Record, error) {
	return dp.accounts.GetByAuthorityAddress(ctx, address)
}
func (dp *DatabaseProvider) GetLatestAccountInfosByOwnerAddress(ctx context.Context, address string) (map[commonpb.AccountType][]*account.Record, error) {
	return dp.accounts.GetLatestByOwnerAddress(ctx, address)
}
func (dp *DatabaseProvider) GetLatestAccountInfoByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (*account.Record, error) {
	return dp.accounts.GetLatestByOwnerAddressAndType(ctx, address, accountType)
}
func (dp *DatabaseProvider) GetRelationshipAccountInfoByOwnerAddress(ctx context.Context, address, relationshipTo string) (*account.Record, error) {
	return dp.accounts.GetRelationshipByOwnerAddress(ctx, address, relationshipTo)
}
func (dp *DatabaseProvider) GetPrioritizedAccountInfosRequiringDepositSync(ctx context.Context, limit uint64) ([]*account.Record, error) {
	return dp.accounts.GetPrioritizedRequiringDepositSync(ctx, limit)
}
func (dp *DatabaseProvider) GetPrioritizedAccountInfosRequiringAutoReturnCheck(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error) {
	return dp.accounts.GetPrioritizedRequiringAutoReturnCheck(ctx, maxAge, limit)
}
func (dp *DatabaseProvider) GetPrioritizedAccountInfosRequiringSwapRetry(ctx context.Context, maxAge time.Duration, limit uint64) ([]*account.Record, error) {
	return dp.accounts.GetPrioritizedRequiringSwapRetry(ctx, maxAge, limit)
}
func (dp *DatabaseProvider) GetAccountInfoCountRequiringDepositSync(ctx context.Context) (uint64, error) {
	return dp.accounts.CountRequiringDepositSync(ctx)
}
func (dp *DatabaseProvider) GetAccountInfoCountRequiringAutoReturnCheck(ctx context.Context) (uint64, error) {
	return dp.accounts.CountRequiringAutoReturnCheck(ctx)
}
func (dp *DatabaseProvider) GetAccountInfoCountRequiringSwapRetry(ctx context.Context) (uint64, error) {
	return dp.accounts.CountRequiringSwapRetry(ctx)
}

// Currency
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetExchangeRate(ctx context.Context, code currency_lib.Code, t time.Time) (*currency.ExchangeRateRecord, error) {
	key := fmt.Sprintf("%s:%s", code, t.Truncate(5*time.Minute).Format(time.RFC3339))
	if rate, ok := dp.exchangeCache.Retrieve(key); ok {
		return rate.(*currency.ExchangeRateRecord), nil
	}

	rate, err := dp.currencies.Get(ctx, string(code), t)
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

	rates, err := dp.currencies.GetAll(ctx, t)
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

	return dp.currencies.GetRange(ctx, string(code), req.Interval, req.Start, req.End, req.SortBy)
}
func (dp *DatabaseProvider) ImportExchangeRates(ctx context.Context, data *currency.MultiRateRecord) error {
	return dp.currencies.Put(ctx, data)
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

// Nonce
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetNonce(ctx context.Context, address string) (*nonce.Record, error) {
	return dp.nonces.Get(ctx, address)
}
func (dp *DatabaseProvider) GetNonceCount(ctx context.Context) (uint64, error) {
	return dp.nonces.Count(ctx)
}
func (dp *DatabaseProvider) GetNonceCountByState(ctx context.Context, state nonce.State) (uint64, error) {
	return dp.nonces.CountByState(ctx, state)
}
func (dp *DatabaseProvider) GetNonceCountByStateAndPurpose(ctx context.Context, state nonce.State, purpose nonce.Purpose) (uint64, error) {
	return dp.nonces.CountByStateAndPurpose(ctx, state, purpose)
}
func (dp *DatabaseProvider) GetAllNonceByState(ctx context.Context, state nonce.State, opts ...query.Option) ([]*nonce.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.nonces.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetRandomAvailableNonceByPurpose(ctx context.Context, purpose nonce.Purpose) (*nonce.Record, error) {
	return dp.nonces.GetRandomAvailableByPurpose(ctx, purpose)
}
func (dp *DatabaseProvider) SaveNonce(ctx context.Context, record *nonce.Record) error {
	return dp.nonces.Save(ctx, record)
}

// Fulfillment
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetFulfillmentById(ctx context.Context, id uint64) (*fulfillment.Record, error) {
	return dp.fulfillments.GetById(ctx, id)
}
func (dp *DatabaseProvider) GetFulfillmentBySignature(ctx context.Context, signature string) (*fulfillment.Record, error) {
	return dp.fulfillments.GetBySignature(ctx, signature)
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
func (dp *DatabaseProvider) MarkFulfillmentAsActivelyScheduled(ctx context.Context, id uint64) error {
	return dp.fulfillments.MarkAsActivelyScheduled(ctx, id)
}
func (dp *DatabaseProvider) ActivelyScheduleTreasuryAdvanceFulfillments(ctx context.Context, treasury string, intentOrderingIndex uint64, limit int) (uint64, error) {
	return dp.fulfillments.ActivelyScheduleTreasuryAdvances(ctx, treasury, intentOrderingIndex, limit)
}

// Intent
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
func (dp *DatabaseProvider) GetLatestIntentByInitiatorAndType(ctx context.Context, intentType intent.Type, owner string) (*intent.Record, error) {
	return dp.intents.GetLatestByInitiatorAndType(ctx, intentType, owner)
}
func (dp *DatabaseProvider) GetAllIntentsByOwner(ctx context.Context, owner string, opts ...query.Option) ([]*intent.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.intents.GetAllByOwner(ctx, owner, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetIntentCountForAntispam(ctx context.Context, intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) (uint64, error) {
	return dp.intents.CountForAntispam(ctx, intentType, phoneNumber, states, since)
}
func (dp *DatabaseProvider) GetIntentCountWithOwnerInteractionsForAntispam(ctx context.Context, sourceOwner, destinationOwner string, states []intent.State, since time.Time) (uint64, error) {
	return dp.intents.CountOwnerInteractionsForAntispam(ctx, sourceOwner, destinationOwner, states, since)
}
func (dp *DatabaseProvider) GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	return dp.intents.GetTransactedAmountForAntiMoneyLaundering(ctx, phoneNumber, since)
}
func (dp *DatabaseProvider) GetDepositedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	return dp.intents.GetDepositedAmountForAntiMoneyLaundering(ctx, phoneNumber, since)
}
func (dp *DatabaseProvider) GetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, account string) (int64, error) {
	return dp.intents.GetNetBalanceFromPrePrivacy2022Intents(ctx, account)
}
func (dp *DatabaseProvider) GetLatestSaveRecentRootIntentForTreasury(ctx context.Context, treasury string) (*intent.Record, error) {
	return dp.intents.GetLatestSaveRecentRootIntentForTreasury(ctx, treasury)
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

// Action
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

// Payment
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetPayment(ctx context.Context, sig string, index int) (*payment.Record, error) {
	return dp.payments.Get(ctx, sig, uint32(index))
}
func (dp *DatabaseProvider) CreatePayment(ctx context.Context, record *payment.Record) error {
	return dp.payments.Put(ctx, record)
}
func (dp *DatabaseProvider) UpdatePayment(ctx context.Context, record *payment.Record) error {
	return dp.payments.Update(ctx, record)
}
func (dp *DatabaseProvider) UpdateOrCreatePayment(ctx context.Context, record *payment.Record) error {
	if record.Id > 0 {
		return dp.UpdatePayment(ctx, record)
	}
	return dp.CreatePayment(ctx, record)
}
func (dp *DatabaseProvider) GetPaymentHistory(ctx context.Context, account string, opts ...query.Option) ([]*payment.Record, error) {
	req := query.QueryOptions{
		Limit:     maxPaymentHistoryReqSize,
		SortBy:    query.Ascending,
		Supported: query.CanLimitResults | query.CanSortBy | query.CanQueryByCursor | query.CanFilterBy,
	}
	req.Apply(opts...)

	if req.Limit > maxPaymentHistoryReqSize {
		return nil, query.ErrQueryNotSupported
	}

	var cursor uint64
	if len(req.Cursor) > 0 {
		cursor = query.FromCursor(req.Cursor)
	} else {
		cursor = 0
	}

	if req.FilterBy.Valid {
		return dp.payments.GetAllForAccountByType(ctx, account, cursor, uint(req.Limit), req.SortBy, payment.PaymentType(req.FilterBy.Value))
	}

	return dp.payments.GetAllForAccount(ctx, account, cursor, uint(req.Limit), req.SortBy)
}
func (dp *DatabaseProvider) GetPaymentHistoryWithinBlockRange(ctx context.Context, account string, lowerBound, upperBound uint64, opts ...query.Option) ([]*payment.Record, error) {
	req := query.QueryOptions{
		Limit:     maxPaymentHistoryReqSize,
		SortBy:    query.Ascending,
		Supported: query.CanLimitResults | query.CanSortBy | query.CanQueryByCursor | query.CanFilterBy,
	}
	req.Apply(opts...)

	if req.Limit > maxPaymentHistoryReqSize {
		return nil, query.ErrQueryNotSupported
	}

	var cursor uint64
	if len(req.Cursor) > 0 {
		cursor = query.FromCursor(req.Cursor)
	} else {
		cursor = 0
	}

	if req.FilterBy.Valid {
		return dp.payments.GetAllForAccountByTypeWithinBlockRange(ctx, account, lowerBound, upperBound, cursor, uint(req.Limit), req.SortBy, payment.PaymentType(req.FilterBy.Value))
	}

	return nil, query.ErrQueryNotSupported
}
func (dp *DatabaseProvider) GetLegacyTotalExternalDepositAmountFromPrePrivacy2022Accounts(ctx context.Context, account string) (uint64, error) {
	return dp.payments.GetExternalDepositAmount(ctx, account)
}

// Transaction
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetTransaction(ctx context.Context, sig string) (*transaction.Record, error) {
	return dp.transactions.Get(ctx, sig)
}
func (dp *DatabaseProvider) SaveTransaction(ctx context.Context, record *transaction.Record) error {
	return dp.transactions.Put(ctx, record)
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

// Phone
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SavePhoneVerification(ctx context.Context, v *phone.Verification) error {
	return dp.phone.SaveVerification(ctx, v)
}
func (dp *DatabaseProvider) GetPhoneVerification(ctx context.Context, account, phoneNumber string) (*phone.Verification, error) {
	return dp.phone.GetVerification(ctx, account, phoneNumber)
}
func (dp *DatabaseProvider) GetLatestPhoneVerificationForAccount(ctx context.Context, account string) (*phone.Verification, error) {
	return dp.phone.GetLatestVerificationForAccount(ctx, account)
}
func (dp *DatabaseProvider) GetLatestPhoneVerificationForNumber(ctx context.Context, phoneNumber string) (*phone.Verification, error) {
	return dp.phone.GetLatestVerificationForNumber(ctx, phoneNumber)
}
func (dp *DatabaseProvider) GetAllPhoneVerificationsForNumber(ctx context.Context, phoneNumber string) ([]*phone.Verification, error) {
	return dp.phone.GetAllVerificationsForNumber(ctx, phoneNumber)
}
func (dp *DatabaseProvider) SavePhoneLinkingToken(ctx context.Context, token *phone.LinkingToken) error {
	return dp.phone.SaveLinkingToken(ctx, token)
}
func (dp *DatabaseProvider) UsePhoneLinkingToken(ctx context.Context, phoneNumber, code string) error {
	return dp.phone.UseLinkingToken(ctx, phoneNumber, code)
}
func (dp *DatabaseProvider) FilterVerifiedPhoneNumbers(ctx context.Context, phoneNumbers []string) ([]string, error) {
	return dp.phone.FilterVerifiedNumbers(ctx, phoneNumbers)
}
func (dp *DatabaseProvider) SaveOwnerAccountPhoneSetting(ctx context.Context, phoneNumber string, newSettings *phone.OwnerAccountSetting) error {
	return dp.phone.SaveOwnerAccountSetting(ctx, phoneNumber, newSettings)
}
func (dp *DatabaseProvider) IsPhoneNumberLinkedToAccount(ctx context.Context, phoneNumber string, ownerAccount string) (bool, error) {
	verification, err := dp.GetLatestPhoneVerificationForNumber(ctx, phoneNumber)
	if err != nil {
		return false, err
	} else if verification.OwnerAccount != ownerAccount {
		return false, nil
	}

	phoneSettings, err := dp.phone.GetSettings(ctx, phoneNumber)
	if err != nil {
		return false, err
	}

	tokenAccountSettings, ok := phoneSettings.ByOwnerAccount[ownerAccount]
	if !ok {
		return true, nil
	}

	if tokenAccountSettings.IsUnlinked == nil {
		return true, nil
	}
	return !*tokenAccountSettings.IsUnlinked, nil
}
func (dp *DatabaseProvider) IsPhoneNumberEnabledForRemoteSendToAccount(ctx context.Context, phoneNumber string, ownerAccount string) (bool, error) {
	// These are equivalent at the time of writing this
	return dp.IsPhoneNumberLinkedToAccount(ctx, phoneNumber, ownerAccount)
}
func (dp *DatabaseProvider) PutPhoneEvent(ctx context.Context, event *phone.Event) error {
	return dp.phone.PutEvent(ctx, event)
}
func (dp *DatabaseProvider) GetLatestPhoneEventForNumberByType(ctx context.Context, phoneNumber string, eventType phone.EventType) (*phone.Event, error) {
	return dp.phone.GetLatestEventForNumberByType(ctx, phoneNumber, eventType)
}
func (dp *DatabaseProvider) GetPhoneEventCountForVerificationByType(ctx context.Context, verification string, eventType phone.EventType) (uint64, error) {
	return dp.phone.CountEventsForVerificationByType(ctx, verification, eventType)
}
func (dp *DatabaseProvider) GetPhoneEventCountForNumberByTypeSinceTimestamp(ctx context.Context, phoneNumber string, eventType phone.EventType, since time.Time) (uint64, error) {
	return dp.phone.CountEventsForNumberByTypeSinceTimestamp(ctx, phoneNumber, eventType, since)
}
func (dp *DatabaseProvider) GetUniquePhoneVerificationIdCountForNumberSinceTimestamp(ctx context.Context, phoneNumber string, since time.Time) (uint64, error) {
	return dp.phone.CountUniqueVerificationIdsForNumberSinceTimestamp(ctx, phoneNumber, since)
}

// Contact
// --------------------------------------------------------------------------------

func (dp *DatabaseProvider) AddContact(ctx context.Context, owner *user.DataContainerID, contact string) error {
	return dp.contact.Add(ctx, owner, contact)
}
func (dp *DatabaseProvider) BatchAddContacts(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	return dp.contact.BatchAdd(ctx, owner, contacts)
}
func (dp *DatabaseProvider) RemoveContact(ctx context.Context, owner *user.DataContainerID, contact string) error {
	return dp.contact.Remove(ctx, owner, contact)
}
func (dp *DatabaseProvider) GetContacts(ctx context.Context, owner *user.DataContainerID, limit uint32, pageToken []byte) (contacts []string, nextPageToken []byte, err error) {
	return dp.contact.Get(ctx, owner, limit, pageToken)
}
func (dp *DatabaseProvider) BatchRemoveContacts(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	return dp.contact.BatchRemove(ctx, owner, contacts)
}

// User Identity
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutUser(ctx context.Context, record *identity.Record) error {
	return dp.userIdentity.Put(ctx, record)
}
func (dp *DatabaseProvider) GetUserByID(ctx context.Context, id *user.UserID) (*identity.Record, error) {
	return dp.userIdentity.GetByID(ctx, id)
}
func (dp *DatabaseProvider) GetUserByPhoneView(ctx context.Context, phoneNumber string) (*identity.Record, error) {
	view := &user.View{
		PhoneNumber: &phoneNumber,
	}
	return dp.userIdentity.GetByView(ctx, view)
}

// User Storage Management
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutUserDataContainer(ctx context.Context, record *storage.Record) error {
	return dp.userStorage.Put(ctx, record)
}
func (dp *DatabaseProvider) GetUserDataContainerByID(ctx context.Context, id *user.DataContainerID) (*storage.Record, error) {
	return dp.userStorage.GetByID(ctx, id)
}
func (dp *DatabaseProvider) GetUserDataContainerByPhone(ctx context.Context, tokenAccount, phoneNumber string) (*storage.Record, error) {
	identifyingFeatures := &user.IdentifyingFeatures{
		PhoneNumber: &phoneNumber,
	}
	return dp.userStorage.GetByFeatures(ctx, tokenAccount, identifyingFeatures)
}

// Timelock
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveTimelock(ctx context.Context, record *timelock.Record) error {
	return dp.timelock.Save(ctx, record)
}
func (dp *DatabaseProvider) GetTimelockByAddress(ctx context.Context, address string) (*timelock.Record, error) {
	// todo: add caching if this becomes a heavy hitter like GetByVault
	return dp.timelock.GetByAddress(ctx, address)
}
func (dp *DatabaseProvider) GetTimelockByVaultBatch(ctx context.Context, vaults ...string) (map[string]*timelock.Record, error) {
	records, err := dp.timelock.GetByVaultBatch(ctx, vaults...)
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
		return dp.timelock.GetByVault(ctx, vault)
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
		record, err := dp.timelock.GetByVault(ctx, vault)
		if err == nil {
			cacheEntry.record = record.Clone()
			cacheEntry.lastUpdatedAt = time.Now()
		}
		return record, err
	}

	// Record not cached, so fetch it and insert the initial cache entry
	record, err := dp.timelock.GetByVault(ctx, vault)
	if err == nil {
		cacheEntry := &timelockCacheEntry{
			record:        record.Clone(),
			lastUpdatedAt: time.Now(),
		}
		dp.timelockCache.Insert(vault, cacheEntry, 1)
	}
	return record, err
}
func (dp *DatabaseProvider) GetAllTimelocksByState(ctx context.Context, state timelock_token.TimelockState, opts ...query.Option) ([]*timelock.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.timelock.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetTimelockCountByState(ctx context.Context, state timelock_token.TimelockState) (uint64, error) {
	return dp.timelock.GetCountByState(ctx, state)
}

// Push
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutPushToken(ctx context.Context, record *push.Record) error {
	return dp.push.Put(ctx, record)
}
func (dp *DatabaseProvider) MarkPushTokenAsInvalid(ctx context.Context, pushToken string) error {
	return dp.push.MarkAsInvalid(ctx, pushToken)
}
func (dp *DatabaseProvider) DeletePushToken(ctx context.Context, pushToken string) error {
	return dp.push.Delete(ctx, pushToken)
}
func (dp *DatabaseProvider) GetAllValidPushTokensdByDataContainer(ctx context.Context, id *user.DataContainerID) ([]*push.Record, error) {
	return dp.push.GetAllValidByDataContainer(ctx, id)
}

// Commitment
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveCommitment(ctx context.Context, record *commitment.Record) error {
	return dp.commitment.Save(ctx, record)
}
func (dp *DatabaseProvider) GetCommitmentByAddress(ctx context.Context, address string) (*commitment.Record, error) {
	return dp.commitment.GetByAddress(ctx, address)
}
func (dp *DatabaseProvider) GetCommitmentByVault(ctx context.Context, vault string) (*commitment.Record, error) {
	return dp.commitment.GetByVault(ctx, vault)
}
func (dp *DatabaseProvider) GetCommitmentByAction(ctx context.Context, intentId string, actionId uint32) (*commitment.Record, error) {
	return dp.commitment.GetByAction(ctx, intentId, actionId)
}
func (dp *DatabaseProvider) GetAllCommitmentsByState(ctx context.Context, state commitment.State, opts ...query.Option) ([]*commitment.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.commitment.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) GetUpgradeableCommitmentsByOwner(ctx context.Context, owner string, limit uint64) ([]*commitment.Record, error) {
	return dp.commitment.GetUpgradeableByOwner(ctx, owner, limit)
}
func (dp *DatabaseProvider) GetUsedTreasuryPoolDeficitFromCommitments(ctx context.Context, treasuryPool string) (uint64, error) {
	return dp.commitment.GetUsedTreasuryPoolDeficit(ctx, treasuryPool)
}
func (dp *DatabaseProvider) GetTotalTreasuryPoolDeficitFromCommitments(ctx context.Context, treasuryPool string) (uint64, error) {
	return dp.commitment.GetTotalTreasuryPoolDeficit(ctx, treasuryPool)
}
func (dp *DatabaseProvider) CountCommitmentsByState(ctx context.Context, state commitment.State) (uint64, error) {
	return dp.commitment.CountByState(ctx, state)
}
func (dp *DatabaseProvider) CountCommitmentRepaymentsDivertedToVault(ctx context.Context, vault string) (uint64, error) {
	return dp.commitment.CountRepaymentsDivertedToVault(ctx, vault)
}

// Treasury Pool
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveTreasuryPool(ctx context.Context, record *treasury.Record) error {
	return dp.treasury.Save(ctx, record)
}
func (dp *DatabaseProvider) GetTreasuryPoolByName(ctx context.Context, name string) (*treasury.Record, error) {
	return dp.treasury.GetByName(ctx, name)
}
func (dp *DatabaseProvider) GetTreasuryPoolByAddress(ctx context.Context, address string) (*treasury.Record, error) {
	return dp.treasury.GetByAddress(ctx, address)
}
func (dp *DatabaseProvider) GetTreasuryPoolByVault(ctx context.Context, vault string) (*treasury.Record, error) {
	return dp.treasury.GetByVault(ctx, vault)
}
func (dp *DatabaseProvider) GetAllTreasuryPoolsByState(ctx context.Context, state treasury.TreasuryPoolState, opts ...query.Option) ([]*treasury.Record, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}

	return dp.treasury.GetAllByState(ctx, state, req.Cursor, req.Limit, req.SortBy)
}
func (dp *DatabaseProvider) SaveTreasuryPoolFunding(ctx context.Context, record *treasury.FundingHistoryRecord) error {
	return dp.treasury.SaveFunding(ctx, record)
}
func (dp *DatabaseProvider) GetTotalAvailableTreasuryPoolFunds(ctx context.Context, vault string) (uint64, error) {
	return dp.treasury.GetTotalAvailableFunds(ctx, vault)
}

// Merkle Tree
func (dp *DatabaseProvider) InitializeNewMerkleTree(ctx context.Context, name string, levels uint8, seeds []merkletree.Seed, readOnly bool) (*merkletree.MerkleTree, error) {
	return merkletree.InitializeNew(ctx, dp.merkleTree, name, levels, seeds, readOnly)
}
func (dp *DatabaseProvider) LoadExistingMerkleTree(ctx context.Context, name string, readOnly bool) (*merkletree.MerkleTree, error) {
	return merkletree.LoadExisting(ctx, dp.merkleTree, name, readOnly)
}

// External Deposits
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

// Rendezvous
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveRendezvous(ctx context.Context, record *rendezvous.Record) error {
	return dp.rendezvous.Save(ctx, record)
}
func (dp *DatabaseProvider) GetRendezvous(ctx context.Context, key string) (*rendezvous.Record, error) {
	return dp.rendezvous.Get(ctx, key)
}
func (dp *DatabaseProvider) DeleteRendezvous(ctx context.Context, key string) error {
	return dp.rendezvous.Delete(ctx, key)
}

// Payment Request
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) CreateRequest(ctx context.Context, record *paymentrequest.Record) error {
	return dp.paymentRequest.Put(ctx, record)
}
func (dp *DatabaseProvider) GetRequest(ctx context.Context, intentId string) (*paymentrequest.Record, error) {
	return dp.paymentRequest.Get(ctx, intentId)
}

// Paywall
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) CreatePaywall(ctx context.Context, record *paywall.Record) error {
	return dp.paywall.Put(ctx, record)
}
func (dp *DatabaseProvider) GetPaywallByShortPath(ctx context.Context, path string) (*paywall.Record, error) {
	return dp.paywall.GetByShortPath(ctx, path)
}

// Event
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveEvent(ctx context.Context, record *event.Record) error {
	return dp.event.Save(ctx, record)
}
func (dp *DatabaseProvider) GetEvent(ctx context.Context, id string) (*event.Record, error) {
	return dp.event.Get(ctx, id)
}

// Webhook
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) CreateWebhook(ctx context.Context, record *webhook.Record) error {
	return dp.webhook.Put(ctx, record)
}
func (dp *DatabaseProvider) UpdateWebhook(ctx context.Context, record *webhook.Record) error {
	return dp.webhook.Update(ctx, record)
}
func (dp *DatabaseProvider) GetWebhook(ctx context.Context, webhookId string) (*webhook.Record, error) {
	return dp.webhook.Get(ctx, webhookId)
}
func (dp *DatabaseProvider) CountWebhookByState(ctx context.Context, state webhook.State) (uint64, error) {
	return dp.webhook.CountByState(ctx, state)
}
func (dp *DatabaseProvider) GetAllPendingWebhooksReadyToSend(ctx context.Context, limit uint64) ([]*webhook.Record, error) {
	return dp.webhook.GetAllPendingReadyToSend(ctx, limit)
}

// Chat V1
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutChatV1(ctx context.Context, record *chat_v1.Chat) error {
	return dp.chatv1.PutChat(ctx, record)
}
func (dp *DatabaseProvider) GetChatByIdV1(ctx context.Context, chatId chat_v1.ChatId) (*chat_v1.Chat, error) {
	return dp.chatv1.GetChatById(ctx, chatId)
}
func (dp *DatabaseProvider) GetAllChatsForUserV1(ctx context.Context, user string, opts ...query.Option) ([]*chat_v1.Chat, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}
	return dp.chatv1.GetAllChatsForUser(ctx, user, req.Cursor, req.SortBy, req.Limit)
}
func (dp *DatabaseProvider) PutChatMessageV1(ctx context.Context, record *chat_v1.Message) error {
	return dp.chatv1.PutMessage(ctx, record)
}
func (dp *DatabaseProvider) DeleteChatMessageV1(ctx context.Context, chatId chat_v1.ChatId, messageId string) error {
	return dp.chatv1.DeleteMessage(ctx, chatId, messageId)
}
func (dp *DatabaseProvider) GetChatMessageV1(ctx context.Context, chatId chat_v1.ChatId, messageId string) (*chat_v1.Message, error) {
	return dp.chatv1.GetMessageById(ctx, chatId, messageId)
}
func (dp *DatabaseProvider) GetAllChatMessagesV1(ctx context.Context, chatId chat_v1.ChatId, opts ...query.Option) ([]*chat_v1.Message, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}
	return dp.chatv1.GetAllMessagesByChat(ctx, chatId, req.Cursor, req.SortBy, req.Limit)
}
func (dp *DatabaseProvider) AdvanceChatPointerV1(ctx context.Context, chatId chat_v1.ChatId, pointer string) error {
	return dp.chatv1.AdvancePointer(ctx, chatId, pointer)
}
func (dp *DatabaseProvider) GetChatUnreadCountV1(ctx context.Context, chatId chat_v1.ChatId) (uint32, error) {
	return dp.chatv1.GetUnreadCount(ctx, chatId)
}
func (dp *DatabaseProvider) SetChatMuteStateV1(ctx context.Context, chatId chat_v1.ChatId, isMuted bool) error {
	return dp.chatv1.SetMuteState(ctx, chatId, isMuted)
}
func (dp *DatabaseProvider) SetChatSubscriptionStateV1(ctx context.Context, chatId chat_v1.ChatId, isSubscribed bool) error {
	return dp.chatv1.SetSubscriptionState(ctx, chatId, isSubscribed)
}

// Chat V2
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) GetChatByIdV2(ctx context.Context, chatId chat_v2.ChatId) (*chat_v2.ChatRecord, error) {
	return dp.chatv2.GetChatById(ctx, chatId)
}
func (dp *DatabaseProvider) GetChatMemberByIdV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId) (*chat_v2.MemberRecord, error) {
	return dp.chatv2.GetMemberById(ctx, chatId, memberId)
}
func (dp *DatabaseProvider) GetChatMessageByIdV2(ctx context.Context, chatId chat_v2.ChatId, messageId chat_v2.MessageId) (*chat_v2.MessageRecord, error) {
	return dp.chatv2.GetMessageById(ctx, chatId, messageId)
}
func (dp *DatabaseProvider) GetAllChatMembersV2(ctx context.Context, chatId chat_v2.ChatId) ([]*chat_v2.MemberRecord, error) {
	return dp.chatv2.GetAllMembersByChatId(ctx, chatId)
}
func (dp *DatabaseProvider) GetPlatformUserChatMembershipV2(ctx context.Context, idByPlatform map[chat_v2.Platform]string, opts ...query.Option) ([]*chat_v2.MemberRecord, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}
	return dp.chatv2.GetAllMembersByPlatformIds(ctx, idByPlatform, req.Cursor, req.SortBy, req.Limit)
}
func (dp *DatabaseProvider) GetAllChatMessagesV2(ctx context.Context, chatId chat_v2.ChatId, opts ...query.Option) ([]*chat_v2.MessageRecord, error) {
	req, err := query.DefaultPaginationHandler(opts...)
	if err != nil {
		return nil, err
	}
	return dp.chatv2.GetAllMessagesByChatId(ctx, chatId, req.Cursor, req.SortBy, req.Limit)
}
func (dp *DatabaseProvider) GetChatUnreadCountV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, readPointer chat_v2.MessageId) (uint32, error) {
	return dp.chatv2.GetUnreadCount(ctx, chatId, memberId, readPointer)
}
func (dp *DatabaseProvider) PutChatV2(ctx context.Context, record *chat_v2.ChatRecord) error {
	return dp.chatv2.PutChat(ctx, record)
}
func (dp *DatabaseProvider) PutChatMemberV2(ctx context.Context, record *chat_v2.MemberRecord) error {
	return dp.chatv2.PutMember(ctx, record)
}
func (dp *DatabaseProvider) PutChatMessageV2(ctx context.Context, record *chat_v2.MessageRecord) error {
	return dp.chatv2.PutMessage(ctx, record)
}
func (dp *DatabaseProvider) AdvanceChatPointerV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, pointerType chat_v2.PointerType, pointer chat_v2.MessageId) (bool, error) {
	return dp.chatv2.AdvancePointer(ctx, chatId, memberId, pointerType, pointer)
}
func (dp *DatabaseProvider) UpgradeChatMemberIdentityV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, platform chat_v2.Platform, platformId string) error {
	return dp.chatv2.UpgradeIdentity(ctx, chatId, memberId, platform, platformId)
}
func (dp *DatabaseProvider) SetChatMuteStateV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, isMuted bool) error {
	return dp.chatv2.SetMuteState(ctx, chatId, memberId, isMuted)
}
func (dp *DatabaseProvider) SetChatSubscriptionStateV2(ctx context.Context, chatId chat_v2.ChatId, memberId chat_v2.MemberId, isSubscribed bool) error {
	return dp.chatv2.SetSubscriptionState(ctx, chatId, memberId, isSubscribed)
}

// Badge Count
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) AddToBadgeCount(ctx context.Context, owner string, amount uint32) error {
	return dp.badgecount.Add(ctx, owner, amount)
}
func (dp *DatabaseProvider) ResetBadgeCount(ctx context.Context, owner string) error {
	return dp.badgecount.Reset(ctx, owner)
}
func (dp *DatabaseProvider) GetBadgeCount(ctx context.Context, owner string) (*badgecount.Record, error) {
	return dp.badgecount.Get(ctx, owner)
}

// Login
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveLogins(ctx context.Context, record *login.MultiRecord) error {
	return dp.login.Save(ctx, record)
}
func (dp *DatabaseProvider) GetLoginsByAppInstall(ctx context.Context, appInstallId string) (*login.MultiRecord, error) {
	return dp.login.GetAllByInstallId(ctx, appInstallId)
}
func (dp *DatabaseProvider) GetLatestLoginByOwner(ctx context.Context, owner string) (*login.Record, error) {
	return dp.login.GetLatestByOwner(ctx, owner)
}

// Balance
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveBalanceCheckpoint(ctx context.Context, record *balance.Record) error {
	return dp.balance.SaveCheckpoint(ctx, record)
}
func (dp *DatabaseProvider) GetBalanceCheckpoint(ctx context.Context, account string) (*balance.Record, error) {
	return dp.balance.GetCheckpoint(ctx, account)
}

// Onramp
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) PutFiatOnrampPurchase(ctx context.Context, record *onramp.Record) error {
	return dp.onramp.Put(ctx, record)
}
func (dp *DatabaseProvider) GetFiatOnrampPurchase(ctx context.Context, nonce uuid.UUID) (*onramp.Record, error) {
	return dp.onramp.Get(ctx, nonce)
}

// User Preferences
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveUserPreferences(ctx context.Context, record *preferences.Record) error {
	return dp.preferences.Save(ctx, record)
}
func (dp *DatabaseProvider) GetUserPreferences(ctx context.Context, id *user.DataContainerID) (*preferences.Record, error) {
	return dp.preferences.Get(ctx, id)
}
func (dp *DatabaseProvider) GetUserLocale(ctx context.Context, owner string) (language.Tag, error) {
	verificationRecord, err := dp.GetLatestPhoneVerificationForAccount(ctx, owner)
	if err != nil {
		return language.Und, errors.Wrap(err, "error getting latest phone verification record")
	}

	dataContainerRecord, err := dp.GetUserDataContainerByPhone(ctx, owner, verificationRecord.PhoneNumber)
	if err != nil {
		return language.Und, errors.Wrap(err, "error getting data container record")
	}

	userPreferencesRecord, err := dp.GetUserPreferences(ctx, dataContainerRecord.ID)
	switch err {
	case nil:
		return userPreferencesRecord.Locale, nil
	case preferences.ErrPreferencesNotFound:
		return preferences.GetDefaultLocale(), nil
	default:
		return language.Und, errors.Wrap(err, "error getting user preferences record")
	}
}

// Airdrop
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) MarkIneligibleForAirdrop(ctx context.Context, owner string) error {
	return dp.airdrop.MarkIneligible(ctx, owner)
}
func (dp *DatabaseProvider) IsEligibleForAirdrop(ctx context.Context, owner string) (bool, error) {
	return dp.airdrop.IsEligible(ctx, owner)
}

// Twitter
// --------------------------------------------------------------------------------
func (dp *DatabaseProvider) SaveTwitterUser(ctx context.Context, record *twitter.Record) error {
	return dp.twitter.SaveUser(ctx, record)
}
func (dp *DatabaseProvider) GetTwitterUserByUsername(ctx context.Context, username string) (*twitter.Record, error) {
	return dp.twitter.GetUserByUsername(ctx, username)
}
func (dp *DatabaseProvider) GetTwitterUserByTipAddress(ctx context.Context, tipAddress string) (*twitter.Record, error) {
	return dp.twitter.GetUserByTipAddress(ctx, tipAddress)
}
func (dp *DatabaseProvider) GetStaleTwitterUsers(ctx context.Context, minAge time.Duration, limit int) ([]*twitter.Record, error) {
	return dp.twitter.GetStaleUsers(ctx, minAge, limit)
}
func (dp *DatabaseProvider) MarkTweetAsProcessed(ctx context.Context, tweetId string) error {
	return dp.twitter.MarkTweetAsProcessed(ctx, tweetId)
}
func (dp *DatabaseProvider) IsTweetProcessed(ctx context.Context, tweetId string) (bool, error) {
	return dp.twitter.IsTweetProcessed(ctx, tweetId)
}
func (dp *DatabaseProvider) MarkTwitterNonceAsUsed(ctx context.Context, tweetId string, nonce uuid.UUID) error {
	return dp.twitter.MarkNonceAsUsed(ctx, tweetId, nonce)
}
