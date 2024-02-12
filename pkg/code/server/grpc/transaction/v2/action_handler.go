package transaction_v2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

// todo: a better name for this lol?
type makeSolanaTransactionResult struct {
	isCreatedOnDemand bool
	txn               *solana.Transaction // Can be null if the transaction is on-demand created at scheduling time

	// Additional metadata to add to the action and fulfillment record, which relates
	// specifically to the transaction that was created.

	fulfillmentType fulfillment.Type

	source      *common.Account
	destination *common.Account

	intentOrderingIndexOverride *uint64
	actionOrderingIndexOverride *uint32
	fulfillmentOrderingIndex    uint32

	disableActiveScheduling bool
}

// BaseActionHandler is a base interface for operation-specific action handlers
//
// Note: Action handlers should load all required state on initialization to
// avoid duplicated work across interface method calls.
type BaseActionHandler interface {
	// GetServerParameter gets the server parameter for the action within the context
	// of the intent.
	GetServerParameter() *transactionpb.ServerParameter

	// OnSaveToDB is a callback when the action is being saved to the DB
	// within the scope of a DB transaction. Additional supporting DB records
	// (ie. not the action or fulfillment records) relevant to the action should
	// be saved here.
	OnSaveToDB(ctx context.Context) error
}

// CreateActionHandler is an interface for creating new actions
type CreateActionHandler interface {
	BaseActionHandler

	// TransactionCount returns the total number of transactions that will be created
	// for the action.
	TransactionCount() int

	// PopulateMetadata populates action metadata into the provided record
	PopulateMetadata(actionRecord *action.Record) error

	// RequiresNonce determines whether a nonce should be acquired for the
	// transaction being created. This should only be false in cases where
	// client signatures are not required and transaction construction can
	// be deferred to scheduling time. The nonce and bh parameters of
	// MakeNewSolanaTransaction will be null.
	RequiresNonce(transactionIndex int) bool

	// MakeNewSolanaTransaction makes a new Solana transaction. Implementations
	// can choose to defer creation until scheduling time. This may be done in
	// cases where the client signature is not required.
	MakeNewSolanaTransaction(
		index int,
		nonce *common.Account,
		bh solana.Blockhash,
	) (*makeSolanaTransactionResult, error)
}

// UpgradeActionHandler is an interface for upgrading existing actions. It's
// assumed we'll only be upgrading a single fulfillment.
type UpgradeActionHandler interface {
	BaseActionHandler

	// GetFulfillmentBeingUpgraded gets the original fulfillment that's being
	// upgraded.
	GetFulfillmentBeingUpgraded() *fulfillment.Record

	// MakeUpgradedSolanaTransaction makes an upgraded Solana transaction
	MakeUpgradedSolanaTransaction(
		nonce *common.Account,
		bh solana.Blockhash,
	) (*makeSolanaTransactionResult, error)
}

type OpenAccountActionHandler struct {
	data code_data.Provider

	accountType      commonpb.AccountType
	timelockAccounts *common.TimelockAccounts

	unsavedAccountInfoRecord *account.Record
	unsavedTimelockRecord    *timelock.Record
}

func NewOpenAccountActionHandler(data code_data.Provider, protoAction *transactionpb.OpenAccountAction, protoMetadata *transactionpb.Metadata) (CreateActionHandler, error) {
	owner, err := common.NewAccountFromProto(protoAction.Owner)
	if err != nil {
		return nil, err
	}

	authority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	timelockAccounts, err := authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	var relationshipTo *string
	switch typed := protoMetadata.Type.(type) {
	case *transactionpb.Metadata_EstablishRelationship:
		relationshipTo = &typed.EstablishRelationship.Relationship.GetDomain().Value
	}

	unsavedAccountInfoRecord := &account.Record{
		OwnerAccount:            owner.PublicKey().ToBase58(),
		AuthorityAccount:        authority.PublicKey().ToBase58(),
		TokenAccount:            timelockAccounts.Vault.PublicKey().ToBase58(),
		MintAccount:             timelockAccounts.Mint.PublicKey().ToBase58(),
		AccountType:             protoAction.AccountType,
		Index:                   protoAction.Index,
		RelationshipTo:          relationshipTo,
		DepositsLastSyncedAt:    time.Now(),
		RequiresDepositSync:     false,
		RequiresAutoReturnCheck: protoAction.AccountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
	}

	unsavedTimelockRecord := timelockAccounts.ToDBRecord()

	return &OpenAccountActionHandler{
		data: data,

		accountType:      protoAction.AccountType,
		timelockAccounts: timelockAccounts,

		unsavedAccountInfoRecord: unsavedAccountInfoRecord,
		unsavedTimelockRecord:    unsavedTimelockRecord,
	}, nil
}

func (h *OpenAccountActionHandler) TransactionCount() int {
	return 1
}

func (h *OpenAccountActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.timelockAccounts.Vault.PublicKey().ToBase58()

	actionRecord.State = action.StatePending

	return nil
}

func (h *OpenAccountActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_OpenAccount{
			OpenAccount: &transactionpb.OpenAccountServerParameter{},
		},
	}
}

func (h *OpenAccountActionHandler) RequiresNonce(index int) bool {
	return false
}

func (h *OpenAccountActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		return &makeSolanaTransactionResult{
			isCreatedOnDemand: true,
			txn:               nil,

			fulfillmentType:          fulfillment.InitializeLockedTimelockAccount,
			source:                   h.timelockAccounts.Vault,
			destination:              nil,
			fulfillmentOrderingIndex: 0,
			disableActiveScheduling:  h.accountType != commonpb.AccountType_PRIMARY, // Non-primary accounts are created on demand after first usage
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *OpenAccountActionHandler) OnSaveToDB(ctx context.Context) error {
	err := h.data.SaveTimelock(ctx, h.unsavedTimelockRecord)
	if err != nil {
		return err
	}

	return h.data.CreateAccountInfo(ctx, h.unsavedAccountInfoRecord)
}

type CloseEmptyAccountActionHandler struct {
	timelockAccounts *common.TimelockAccounts
	intentType       intent.Type
}

func NewCloseEmptyAccountActionHandler(intentType intent.Type, protoAction *transactionpb.CloseEmptyAccountAction) (CreateActionHandler, error) {
	authority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	dataVersion := timelock_token_v1.DataVersion1
	if intentType == intent.MigrateToPrivacy2022 {
		dataVersion = timelock_token_v1.DataVersionLegacy
	}
	timelockAccounts, err := authority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	return &CloseEmptyAccountActionHandler{
		timelockAccounts: timelockAccounts,
		intentType:       intentType,
	}, nil
}

func (h *CloseEmptyAccountActionHandler) TransactionCount() int {
	return 1
}

func (h *CloseEmptyAccountActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.timelockAccounts.Vault.PublicKey().ToBase58()

	actionRecord.State = action.StatePending

	return nil
}

func (h *CloseEmptyAccountActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_CloseEmptyAccount{
			CloseEmptyAccount: &transactionpb.CloseEmptyAccountServerParameter{},
		},
	}
}

func (h *CloseEmptyAccountActionHandler) RequiresNonce(index int) bool {
	return true
}

func (h *CloseEmptyAccountActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		txn, err := transaction_util.MakeCloseEmptyAccountTransaction(
			nonce,
			bh,
			h.timelockAccounts,
		)
		if err != nil {
			return nil, err
		}

		return &makeSolanaTransactionResult{
			txn: &txn,

			fulfillmentType:          fulfillment.CloseEmptyTimelockAccount,
			source:                   h.timelockAccounts.Vault,
			destination:              nil,
			fulfillmentOrderingIndex: 0,
			disableActiveScheduling:  h.intentType == intent.ReceivePaymentsPrivately,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *CloseEmptyAccountActionHandler) OnSaveToDB(ctx context.Context) error {
	return nil
}

type CloseDormantAccountActionHandler struct {
	accountType commonpb.AccountType
	source      *common.TimelockAccounts
	destination *common.Account
}

func NewCloseDormantAccountActionHandler(protoAction *transactionpb.CloseDormantAccountAction) (CreateActionHandler, error) {
	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &CloseDormantAccountActionHandler{
		accountType: protoAction.AccountType,
		source:      source,
		destination: destination,
	}, nil
}

func (h *CloseDormantAccountActionHandler) TransactionCount() int {
	return 1
}

func (h *CloseDormantAccountActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	// All actions are revoked, except for those that perform the gift card auto-return
	//
	// Important Note: Given the critical implications of closing a dormant account,
	// especially as an accident due to a bug, ensure proper safeguards in the
	// fulfillment scheduler exist. See commented code in that file.
	initialState := action.StateRevoked
	if h.accountType == commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		initialState = action.StateUnknown
	}

	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()

	destination := h.destination.PublicKey().ToBase58()
	actionRecord.Destination = &destination

	// Do not populat a quantity. This will be done later when we decide to schedule
	// the action. Otherwise, the balance calculator will be completely off. Also, it's
	// not clear what the end balance will be, since this action is reserved for the
	// future when the balance state will likely have changed.
	actionRecord.Quantity = nil

	actionRecord.State = initialState

	return nil
}

func (h *CloseDormantAccountActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_CloseDormantAccount{
			CloseDormantAccount: &transactionpb.CloseDormantAccountServerParameter{},
		},
	}
}

func (h *CloseDormantAccountActionHandler) RequiresNonce(index int) bool {
	return true
}

func (h *CloseDormantAccountActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		txn, err := transaction_util.MakeCloseAccountWithBalanceTransaction(
			nonce,
			bh,
			h.source,
			h.destination,
		)
		if err != nil {
			return nil, err
		}

		intentOrderingIndex := uint64(math.MaxInt64)
		actionOrderingIndex := uint32(0)

		return &makeSolanaTransactionResult{
			txn: &txn,

			fulfillmentType:             fulfillment.CloseDormantTimelockAccount,
			source:                      h.source.Vault,
			destination:                 h.destination,
			intentOrderingIndexOverride: &intentOrderingIndex,
			actionOrderingIndexOverride: &actionOrderingIndex,
			fulfillmentOrderingIndex:    0,
			disableActiveScheduling:     true,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *CloseDormantAccountActionHandler) OnSaveToDB(ctx context.Context) error {
	return nil
}

type NoPrivacyTransferActionHandler struct {
	source       *common.TimelockAccounts
	destination  *common.Account
	amount       uint64
	isFeePayment bool // Internally, the mechanics of a fee payment are exactly the same
}

func NewNoPrivacyTransferActionHandler(protoAction *transactionpb.NoPrivacyTransferAction) (CreateActionHandler, error) {
	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyTransferActionHandler{
		source:       source,
		destination:  destination,
		amount:       protoAction.Amount,
		isFeePayment: false,
	}, nil
}

func NewFeePaymentActionHandler(protoAction *transactionpb.FeePaymentAction, feeCollector *common.Account) (CreateActionHandler, error) {
	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyTransferActionHandler{
		source:       source,
		destination:  feeCollector,
		amount:       protoAction.Amount,
		isFeePayment: true,
	}, nil
}

func (h *NoPrivacyTransferActionHandler) TransactionCount() int {
	return 1
}

func (h *NoPrivacyTransferActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()

	destination := h.destination.PublicKey().ToBase58()
	actionRecord.Destination = &destination

	actionRecord.Quantity = &h.amount

	actionRecord.State = action.StatePending

	return nil
}
func (h *NoPrivacyTransferActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	if h.isFeePayment {
		return &transactionpb.ServerParameter{
			Type: &transactionpb.ServerParameter_FeePayment{
				FeePayment: &transactionpb.FeePaymentServerParameter{
					CodeDestination: h.destination.ToProto(),
				},
			},
		}
	}

	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_NoPrivacyTransfer{
			NoPrivacyTransfer: &transactionpb.NoPrivacyTransferServerParameter{},
		},
	}
}

func (h *NoPrivacyTransferActionHandler) RequiresNonce(index int) bool {
	return true
}

func (h *NoPrivacyTransferActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
			nonce,
			bh,
			h.source,
			h.destination,
			h.amount,
		)
		if err != nil {
			return nil, err
		}

		return &makeSolanaTransactionResult{
			txn: &txn,

			fulfillmentType:          fulfillment.NoPrivacyTransferWithAuthority,
			source:                   h.source.Vault,
			destination:              h.destination,
			fulfillmentOrderingIndex: 0,
			disableActiveScheduling:  h.isFeePayment,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *NoPrivacyTransferActionHandler) OnSaveToDB(ctx context.Context) error {
	return nil
}

type NoPrivacyWithdrawActionHandler struct {
	source      *common.TimelockAccounts
	destination *common.Account
	amount      uint64
	intentType  intent.Type
}

func NewNoPrivacyWithdrawActionHandler(intentType intent.Type, protoAction *transactionpb.NoPrivacyWithdrawAction) (CreateActionHandler, error) {
	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	dataVersion := timelock_token_v1.DataVersion1
	if intentType == intent.MigrateToPrivacy2022 {
		dataVersion = timelock_token_v1.DataVersionLegacy
	}
	source, err := sourceAuthority.GetTimelockAccounts(dataVersion, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyWithdrawActionHandler{
		source:      source,
		destination: destination,
		amount:      protoAction.Amount,
		intentType:  intentType,
	}, nil
}

func (h *NoPrivacyWithdrawActionHandler) TransactionCount() int {
	return 1
}

func (h *NoPrivacyWithdrawActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()

	destination := h.destination.PublicKey().ToBase58()
	actionRecord.Destination = &destination

	actionRecord.Quantity = &h.amount

	actionRecord.State = action.StatePending

	return nil
}
func (h *NoPrivacyWithdrawActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawServerParameter{},
		},
	}
}

func (h *NoPrivacyWithdrawActionHandler) RequiresNonce(index int) bool {
	return true
}

func (h *NoPrivacyWithdrawActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		txn, err := transaction_util.MakeCloseAccountWithBalanceTransaction(
			nonce,
			bh,
			h.source,
			h.destination,
		)
		if err != nil {
			return nil, err
		}

		return &makeSolanaTransactionResult{
			txn: &txn,

			fulfillmentType:          fulfillment.NoPrivacyWithdraw,
			source:                   h.source.Vault,
			destination:              h.destination,
			fulfillmentOrderingIndex: 0,

			// Technically we should do this for public receives too, but we don't
			// yet have a great way of doing cross intent fulfillment polling hints.
			disableActiveScheduling: h.intentType == intent.SendPrivatePayment,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *NoPrivacyWithdrawActionHandler) OnSaveToDB(ctx context.Context) error {
	return nil
}

// Handles both of the equivalent client transfer and exchange actions. The
// server-defined action only defines the private movement of funds between
// accounts and it's all treated the same by backend processes. The client
// definitions are merely metadata to tell us more about the reasoning of
// the movement of funds.
type TemporaryPrivacyTransferActionHandler struct {
	data code_data.Provider

	source            *common.TimelockAccounts
	destination       *common.Account
	treasuryPool      *common.Account
	treasuryPoolVault *common.Account
	commitment        *common.Account
	commitmentVault   *common.Account

	recentRoot merkletree.Hash
	transcript []byte

	unsavedCommitmentRecord *commitment.Record

	isExchange bool

	isCollectedForHideInTheCrowdPrivacy bool
}

func NewTemporaryPrivacyTransferActionHandler(
	ctx context.Context,
	conf *conf,
	data code_data.Provider,
	intentRecord *intent.Record,
	untypedAction *transactionpb.Action,
	isExchange bool,
	treasuryPoolSelector func(context.Context, uint64) (string, error),
) (CreateActionHandler, error) {
	var authorityProto *commonpb.SolanaAccountId
	var destinationProto *commonpb.SolanaAccountId
	var amount uint64
	isCollectedForHideInTheCrowdPrivacy := true
	if isExchange {
		typedAction := untypedAction.GetTemporaryPrivacyExchange()
		if typedAction == nil {
			return nil, errors.New("invalid proto action")
		}
		authorityProto = typedAction.Authority
		destinationProto = typedAction.Destination
		amount = typedAction.Amount
	} else {
		typedAction := untypedAction.GetTemporaryPrivacyTransfer()
		if typedAction == nil {
			return nil, errors.New("invalid proto action")
		}
		authorityProto = typedAction.Authority
		destinationProto = typedAction.Destination
		amount = typedAction.Amount

		// Private payment withdrawals bypass collection state for hide in the
		// crowd privacy. They need to be sent immediately to fulfill withdrawal
		// requirements.
		if intentRecord.IntentType == intent.SendPrivatePayment && intentRecord.SendPrivatePaymentMetadata.IsWithdrawal {
			isCollectedForHideInTheCrowdPrivacy = false
		}
	}

	h := &TemporaryPrivacyTransferActionHandler{
		data:                                data,
		isExchange:                          isExchange,
		isCollectedForHideInTheCrowdPrivacy: isCollectedForHideInTheCrowdPrivacy,
	}

	authority, err := common.NewAccountFromProto(authorityProto)
	if err != nil {
		return nil, err
	}

	h.source, err = authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	h.destination, err = common.NewAccountFromProto(destinationProto)
	if err != nil {
		return nil, err
	}

	selectedTreasuryPoolName, err := treasuryPoolSelector(ctx, amount)
	if err != nil {
		return nil, err
	}

	cachedTreasuryMetadata, err := getCachedTreasuryMetadataByNameOrAddress(ctx, h.data, selectedTreasuryPoolName, conf.treasuryPoolRecentRootCacheMaxAge.Get(ctx))
	if err != nil {
		return nil, err
	}

	h.treasuryPool = cachedTreasuryMetadata.stateAccount
	h.treasuryPoolVault = cachedTreasuryMetadata.vaultAccount

	h.recentRoot, err = hex.DecodeString(cachedTreasuryMetadata.mostRecentRoot)
	if err != nil {
		return nil, err
	}

	h.transcript = getTransript(
		intentRecord.IntentId,
		untypedAction.Id,
		h.source.Vault,
		h.destination,
		amount,
	)

	commitmentAddress, commitmentBump, err := splitter_token.GetCommitmentStateAddress(&splitter_token.GetCommitmentStateAddressArgs{
		Pool:        h.treasuryPool.PublicKey().ToBytes(),
		RecentRoot:  []byte(h.recentRoot),
		Transcript:  h.transcript,
		Destination: h.destination.PublicKey().ToBytes(),
		Amount:      amount,
	})
	if err != nil {
		return nil, err
	}
	h.commitment, err = common.NewAccountFromPublicKeyBytes(commitmentAddress)
	if err != nil {
		return nil, err
	}

	commitmentVaultAddress, commitmentVaultBump, err := splitter_token.GetCommitmentVaultAddress(&splitter_token.GetCommitmentVaultAddressArgs{
		Pool:       h.treasuryPool.PublicKey().ToBytes(),
		Commitment: h.commitment.PublicKey().ToBytes(),
	})
	if err != nil {
		return nil, err
	}
	h.commitmentVault, err = common.NewAccountFromPublicKeyBytes(commitmentVaultAddress)
	if err != nil {
		return nil, err
	}

	h.unsavedCommitmentRecord = &commitment.Record{
		DataVersion: splitter_token.DataVersion1,

		Address: h.commitment.PublicKey().ToBase58(),
		Bump:    commitmentBump,

		Vault:     h.commitmentVault.PublicKey().ToBase58(),
		VaultBump: commitmentVaultBump,

		Pool:     h.treasuryPool.PublicKey().ToBase58(),
		PoolBump: cachedTreasuryMetadata.stateBump,

		RecentRoot: cachedTreasuryMetadata.mostRecentRoot,
		Transcript: hex.EncodeToString(h.transcript),

		Destination: h.destination.PublicKey().ToBase58(),
		Amount:      amount,

		Intent:   intentRecord.IntentId,
		ActionId: untypedAction.Id,
		Owner:    intentRecord.InitiatorOwnerAccount,

		State: commitment.StateUnknown,
	}

	return h, nil
}

func (h *TemporaryPrivacyTransferActionHandler) TransactionCount() int {
	return 2
}

func (h *TemporaryPrivacyTransferActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()
	actionRecord.Destination = &h.unsavedCommitmentRecord.Destination
	actionRecord.Quantity = &h.unsavedCommitmentRecord.Amount

	actionRecord.State = action.StatePending

	return nil
}

func (h *TemporaryPrivacyTransferActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	if h.isExchange {
		return &transactionpb.ServerParameter{
			Type: &transactionpb.ServerParameter_TemporaryPrivacyExchange{
				TemporaryPrivacyExchange: &transactionpb.TemporaryPrivacyExchangeServerParameter{
					Treasury: h.treasuryPool.ToProto(),
					RecentRoot: &commonpb.Hash{
						Value: h.recentRoot,
					},
				},
			},
		}
	}

	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_TemporaryPrivacyTransfer{
			TemporaryPrivacyTransfer: &transactionpb.TemporaryPrivacyTransferServerParameter{
				Treasury: h.treasuryPool.ToProto(),
				RecentRoot: &commonpb.Hash{
					Value: h.recentRoot,
				},
			},
		},
	}
}

func (h *TemporaryPrivacyTransferActionHandler) RequiresNonce(index int) bool {
	return index != 0
}

func (h *TemporaryPrivacyTransferActionHandler) MakeNewSolanaTransaction(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	switch index {
	case 0:
		return &makeSolanaTransactionResult{
			isCreatedOnDemand: true,
			txn:               nil,

			fulfillmentType:          fulfillment.TransferWithCommitment,
			source:                   h.treasuryPoolVault,
			destination:              h.destination,
			fulfillmentOrderingIndex: 0,
			disableActiveScheduling:  h.isCollectedForHideInTheCrowdPrivacy,
		}, nil
	case 1:
		txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
			nonce,
			bh,
			h.source,
			h.commitmentVault,
			h.unsavedCommitmentRecord.Amount,
		)
		if err != nil {
			return nil, err
		}

		return &makeSolanaTransactionResult{
			txn: &txn,

			fulfillmentType:          fulfillment.TemporaryPrivacyTransferWithAuthority,
			source:                   h.source.Vault,
			destination:              h.commitmentVault,
			fulfillmentOrderingIndex: 2000,
			disableActiveScheduling:  true,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *TemporaryPrivacyTransferActionHandler) OnSaveToDB(ctx context.Context) error {
	return h.data.SaveCommitment(ctx, h.unsavedCommitmentRecord)
}

// Handles both of the equivalent client transfer and exchange actions. The
// server-defined action only defines the private movement of funds between
// accounts and it's all treated the same by backend processes. The client
// definitions are merely metadata to tell us more about the reasoning of
// the movement of funds.
type PermanentPrivacyUpgradeActionHandler struct {
	data code_data.Provider

	source              *common.TimelockAccounts
	oldCommitmentVault  *common.Account
	privacyUpgradeProof *privacyUpgradeProof
	amount              uint64

	fulfillmentToUpgrade *fulfillment.Record
}

func NewPermanentPrivacyUpgradeActionHandler(
	ctx context.Context,
	data code_data.Provider,
	intentRecord *intent.Record,
	protoAction *transactionpb.PermanentPrivacyUpgradeAction,
	cachedUpgradeTarget *privacyUpgradeCandidate,
) (UpgradeActionHandler, error) {
	h := &PermanentPrivacyUpgradeActionHandler{
		data: data,
	}

	var err error
	h.fulfillmentToUpgrade, err = h.getFulfillmentBeingUpgraded(ctx, intentRecord, protoAction)
	if err != nil {
		return nil, err
	}

	var txnToUpgrade solana.Transaction
	err = txnToUpgrade.Unmarshal(h.fulfillmentToUpgrade.Data)
	if err != nil {
		return nil, err
	}

	oldIxnArgs, oldIxnAccounts, err := timelock_token_v1.TransferWithAuthorityInstructionFromLegacyInstruction(txnToUpgrade, 2)
	if err != nil {
		return nil, err
	}

	authority, err := common.NewAccountFromPublicKeyBytes(oldIxnAccounts.VaultOwner)
	if err != nil {
		return nil, err
	}

	h.source, err = authority.GetTimelockAccounts(timelock_token_v1.DataVersion1, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	h.oldCommitmentVault, err = common.NewAccountFromPublicKeyBytes(oldIxnAccounts.Destination)
	if err != nil {
		return nil, err
	}

	h.privacyUpgradeProof, err = getProofForPrivacyUpgrade(ctx, h.data, cachedUpgradeTarget)
	if err != nil {
		return nil, err
	}

	h.amount = oldIxnArgs.Amount

	return h, nil
}

func (h *PermanentPrivacyUpgradeActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	protoProof := make([]*commonpb.Hash, len(h.privacyUpgradeProof.proof))
	for i, hash := range h.privacyUpgradeProof.proof {
		protoProof[i] = &commonpb.Hash{
			Value: hash,
		}
	}

	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_PermanentPrivacyUpgrade{
			PermanentPrivacyUpgrade: &transactionpb.PermanentPrivacyUpgradeServerParameter{
				NewCommitment: h.privacyUpgradeProof.newCommitment.ToProto(),
				NewCommitmentTranscript: &commonpb.Hash{
					Value: h.privacyUpgradeProof.newCommitmentTranscript,
				},
				NewCommitmentDestination: h.privacyUpgradeProof.newCommitmentDestination.ToProto(),
				NewCommitmentAmount:      h.privacyUpgradeProof.newCommitmentAmount,
				MerkleRoot: &commonpb.Hash{
					Value: h.privacyUpgradeProof.newCommitmentRoot,
				},
				MerkleProof: protoProof,
			},
		},
	}
}

func (h *PermanentPrivacyUpgradeActionHandler) GetFulfillmentBeingUpgraded() *fulfillment.Record {
	return h.fulfillmentToUpgrade
}

func (h *PermanentPrivacyUpgradeActionHandler) getFulfillmentBeingUpgraded(ctx context.Context, intentRecord *intent.Record, protoAction *transactionpb.PermanentPrivacyUpgradeAction) (*fulfillment.Record, error) {
	fulfillmentRecords, err := h.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, intentRecord.IntentId, protoAction.ActionId)
	if err != nil {
		return nil, err
	}

	if len(fulfillmentRecords) != 1 {
		return nil, errors.New("fulfillment to upgrade was not found")
	}

	return fulfillmentRecords[0], nil
}

func (h *PermanentPrivacyUpgradeActionHandler) MakeUpgradedSolanaTransaction(
	nonce *common.Account,
	bh solana.Blockhash,
) (*makeSolanaTransactionResult, error) {
	txn, err := transaction_util.MakeTransferWithAuthorityTransaction(
		nonce,
		bh,
		h.source,
		h.privacyUpgradeProof.newCommitmentVault,
		h.amount,
	)
	if err != nil {
		return nil, err
	}

	return &makeSolanaTransactionResult{
		txn: &txn,

		fulfillmentType:          fulfillment.PermanentPrivacyTransferWithAuthority,
		source:                   h.source.Vault,
		destination:              h.privacyUpgradeProof.newCommitmentVault,
		fulfillmentOrderingIndex: 1000,
	}, nil
}

func (h *PermanentPrivacyUpgradeActionHandler) OnSaveToDB(ctx context.Context) error {
	commitmentBeingUpgraded, err := h.data.GetCommitmentByVault(ctx, h.oldCommitmentVault.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	newDestination := h.privacyUpgradeProof.newCommitmentVault.PublicKey().ToBase58()
	commitmentBeingUpgraded.RepaymentDivertedTo = &newDestination

	return h.data.SaveCommitment(ctx, commitmentBeingUpgraded)
}

func getTransript(
	intent string,
	action uint32,
	source *common.Account,
	destination *common.Account,
	kinAmountInQuarks uint64,
) []byte {
	transcript := fmt.Sprintf(
		"receipt[%s, %d]: transfer %d quarks from %s to %s",
		intent,
		action,
		kinAmountInQuarks,
		source.PublicKey().ToBase58(),
		destination.PublicKey().ToBase58(),
	)

	hasher := sha256.New()
	hasher.Write([]byte(transcript))
	return hasher.Sum(nil)
}
