package transaction_v2

import (
	"context"
	"errors"
	"math"
	"time"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type newFulfillmentMetadata struct {
	// Signature metadata

	requiresClientSignature bool
	expectedSigner          *common.Account     // Must be null if the requiresClientSignature is false
	virtualIxnHash          *cvm.CompactMessage // Must be null if the requiresClientSignature is false

	// Additional metadata to add to the action and fulfillment record, which relates
	// specifically to the transaction or virtual instruction within the context of
	// the action.

	fulfillmentType fulfillment.Type

	source      *common.Account
	destination *common.Account

	intentOrderingIndexOverride *uint64
	actionOrderingIndexOverride *uint32
	fulfillmentOrderingIndex    uint32
	disableActiveScheduling     bool
}

// CreateActionHandler is an interface for creating new actions
type CreateActionHandler interface {
	// FulfillmentCount returns the total number of fulfillments that
	// will be created for the action.
	FulfillmentCount() int

	// PopulateMetadata populates action metadata into the provided record
	PopulateMetadata(actionRecord *action.Record) error

	// GetServerParameter gets the server parameter for the action within the context
	// of the intent.
	GetServerParameter() *transactionpb.ServerParameter

	// RequiresNonce determines whether a nonce should be acquired for the
	// fulfillment being created. This should be true whenever a virtual
	// instruction needs to be signed by the client. The VM where the action
	// will take place is provided when the result is true.
	RequiresNonce(fulfillmentIndex int) (bool, *common.Account)

	// GetFulfillmentMetadata gets metadata for the fulfillment being created
	GetFulfillmentMetadata(
		index int,
		nonce *common.Account,
		bh solana.Blockhash,
	) (*newFulfillmentMetadata, error)

	// OnCommitToDB is a callback when the action is being committeds to the DB
	// within the scope of a DB transaction. Additional supporting DB records
	// (ie. not the action or fulfillment records) relevant to the action should
	// be saved here.
	OnCommitToDB(ctx context.Context) error
}

type OpenAccountActionHandler struct {
	data code_data.Provider

	accountType      commonpb.AccountType
	timelockAccounts *common.TimelockAccounts

	unsavedAccountInfoRecord *account.Record
	unsavedTimelockRecord    *timelock.Record
}

func NewOpenAccountActionHandler(ctx context.Context, data code_data.Provider, protoAction *transactionpb.OpenAccountAction, protoMetadata *transactionpb.Metadata) (CreateActionHandler, error) {
	mint, err := common.GetBackwardsCompatMint(protoAction.Mint)
	if err != nil {
		return nil, err
	}

	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return nil, err
	}

	owner, err := common.NewAccountFromProto(protoAction.Owner)
	if err != nil {
		return nil, err
	}

	authority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	timelockAccounts, err := authority.GetTimelockAccounts(vmConfig)
	if err != nil {
		return nil, err
	}

	unsavedAccountInfoRecord := &account.Record{
		OwnerAccount:            owner.PublicKey().ToBase58(),
		AuthorityAccount:        authority.PublicKey().ToBase58(),
		TokenAccount:            timelockAccounts.Vault.PublicKey().ToBase58(),
		MintAccount:             timelockAccounts.Mint.PublicKey().ToBase58(),
		AccountType:             protoAction.AccountType,
		Index:                   protoAction.Index,
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

func (h *OpenAccountActionHandler) FulfillmentCount() int {
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

func (h *OpenAccountActionHandler) RequiresNonce(index int) (bool, *common.Account) {
	return false, nil
}

func (h *OpenAccountActionHandler) GetFulfillmentMetadata(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*newFulfillmentMetadata, error) {
	switch index {
	case 0:
		return &newFulfillmentMetadata{
			requiresClientSignature: false,
			expectedSigner:          nil,
			virtualIxnHash:          nil,

			fulfillmentType:          fulfillment.InitializeLockedTimelockAccount,
			source:                   h.timelockAccounts.Vault,
			destination:              nil,
			fulfillmentOrderingIndex: 0,
			disableActiveScheduling:  h.accountType != commonpb.AccountType_PRIMARY, // Non-primary accounts are created on demand after first usage
		}, nil
	default:
		return nil, errors.New("invalid virtual ixn index")
	}
}

func (h *OpenAccountActionHandler) OnCommitToDB(ctx context.Context) error {
	err := h.data.SaveTimelock(ctx, h.unsavedTimelockRecord)
	if err != nil {
		return err
	}

	return h.data.CreateAccountInfo(ctx, h.unsavedAccountInfoRecord)
}

type NoPrivacyTransferActionHandler struct {
	source      *common.TimelockAccounts
	destination *common.Account
	amount      uint64
	feeType     transactionpb.FeePaymentAction_FeeType // Internally, the mechanics of a fee payment are exactly the same
}

func NewNoPrivacyTransferActionHandler(ctx context.Context, data code_data.Provider, protoAction *transactionpb.NoPrivacyTransferAction) (CreateActionHandler, error) {
	mint, err := common.GetBackwardsCompatMint(protoAction.Mint)
	if err != nil {
		return nil, err
	}

	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return nil, err
	}

	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(vmConfig)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyTransferActionHandler{
		source:      source,
		destination: destination,
		amount:      protoAction.Amount,
	}, nil
}

func NewFeePaymentActionHandler(ctx context.Context, data code_data.Provider, protoAction *transactionpb.FeePaymentAction, feeCollector *common.Account) (CreateActionHandler, error) {
	mint, err := common.GetBackwardsCompatMint(protoAction.Mint)
	if err != nil {
		return nil, err
	}

	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return nil, err
	}

	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(vmConfig)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyTransferActionHandler{
		source:      source,
		destination: feeCollector,
		amount:      protoAction.Amount,
		feeType:     protoAction.Type,
	}, nil
}

func (h *NoPrivacyTransferActionHandler) FulfillmentCount() int {
	return 1
}

func (h *NoPrivacyTransferActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()

	actionRecord.Destination = pointer.String(h.destination.PublicKey().ToBase58())

	actionRecord.Quantity = &h.amount

	if h.isFeePayment() {
		actionRecord.FeeType = &h.feeType
	}

	actionRecord.State = action.StatePending

	return nil
}
func (h *NoPrivacyTransferActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	if h.isFeePayment() {
		return &transactionpb.ServerParameter{
			Type: &transactionpb.ServerParameter_FeePayment{
				FeePayment: &transactionpb.FeePaymentServerParameter{
					Destination: h.destination.ToProto(),
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

func (h *NoPrivacyTransferActionHandler) RequiresNonce(index int) (bool, *common.Account) {
	return true, h.source.Vm
}

func (h *NoPrivacyTransferActionHandler) GetFulfillmentMetadata(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*newFulfillmentMetadata, error) {
	switch index {
	case 0:
		virtualIxnHash := cvm.GetCompactTransferMessage(&cvm.GetCompactTransferMessageArgs{
			Source:       h.source.Vault.PublicKey().ToBytes(),
			Destination:  h.destination.PublicKey().ToBytes(),
			Amount:       h.amount,
			NonceAddress: nonce.PublicKey().ToBytes(),
			NonceValue:   cvm.Hash(bh),
		})

		return &newFulfillmentMetadata{
			requiresClientSignature: true,
			expectedSigner:          h.source.VaultOwner,
			virtualIxnHash:          &virtualIxnHash,

			fulfillmentType:          fulfillment.NoPrivacyTransferWithAuthority,
			source:                   h.source.Vault,
			destination:              h.destination,
			fulfillmentOrderingIndex: 0,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *NoPrivacyTransferActionHandler) OnCommitToDB(ctx context.Context) error {
	return nil
}

func (h *NoPrivacyTransferActionHandler) isFeePayment() bool {
	return h.feeType != transactionpb.FeePaymentAction_UNKNOWN
}

type NoPrivacyWithdrawActionHandler struct {
	source       *common.TimelockAccounts
	destination  *common.Account
	amount       uint64
	isAutoReturn bool
}

func NewNoPrivacyWithdrawActionHandler(ctx context.Context, data code_data.Provider, intentRecord *intent.Record, protoAction *transactionpb.NoPrivacyWithdrawAction) (CreateActionHandler, error) {
	mint, err := common.GetBackwardsCompatMint(protoAction.Mint)
	if err != nil {
		return nil, err
	}

	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return nil, err
	}

	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(vmConfig)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyWithdrawActionHandler{
		source:       source,
		destination:  destination,
		amount:       protoAction.Amount,
		isAutoReturn: protoAction.IsAutoReturn,
	}, nil
}

func (h *NoPrivacyWithdrawActionHandler) FulfillmentCount() int {
	return 1
}

func (h *NoPrivacyWithdrawActionHandler) PopulateMetadata(actionRecord *action.Record) error {
	actionRecord.Source = h.source.Vault.PublicKey().ToBase58()

	actionRecord.Destination = pointer.String(h.destination.PublicKey().ToBase58())

	actionRecord.Quantity = &h.amount

	actionRecord.State = action.StatePending

	if h.isAutoReturn {
		// Do not populate a quantity. This will be done later when we decide to schedule
		// the action. Otherwise, the balance calculator will be completely off. Balance
		// amount will be determined at time of scheduling
		actionRecord.Quantity = nil

		actionRecord.State = action.StateUnknown
	}

	return nil
}
func (h *NoPrivacyWithdrawActionHandler) GetServerParameter() *transactionpb.ServerParameter {
	return &transactionpb.ServerParameter{
		Type: &transactionpb.ServerParameter_NoPrivacyWithdraw{
			NoPrivacyWithdraw: &transactionpb.NoPrivacyWithdrawServerParameter{},
		},
	}
}

func (h *NoPrivacyWithdrawActionHandler) RequiresNonce(index int) (bool, *common.Account) {
	return true, h.source.Vm
}

func (h *NoPrivacyWithdrawActionHandler) GetFulfillmentMetadata(
	index int,
	nonce *common.Account,
	bh solana.Blockhash,
) (*newFulfillmentMetadata, error) {
	switch index {
	case 0:
		virtualIxnHash := cvm.GetCompactWithdrawMessage(&cvm.GetCompactWithdrawMessageArgs{
			Source:       h.source.Vault.PublicKey().ToBytes(),
			Destination:  h.destination.PublicKey().ToBytes(),
			NonceAddress: nonce.PublicKey().ToBytes(),
			NonceValue:   cvm.Hash(bh),
		})

		var intentOrderingIndexOverride *uint64
		var actionOrderingIndexOverride *uint32
		if h.isAutoReturn {
			intentOrderingIndexOverride = pointer.Uint64(math.MaxInt64)
			actionOrderingIndexOverride = pointer.Uint32(0)
		}

		return &newFulfillmentMetadata{
			requiresClientSignature: true,
			expectedSigner:          h.source.VaultOwner,
			virtualIxnHash:          &virtualIxnHash,

			fulfillmentType: fulfillment.NoPrivacyWithdraw,
			source:          h.source.Vault,
			destination:     h.destination,

			intentOrderingIndexOverride: intentOrderingIndexOverride,
			actionOrderingIndexOverride: actionOrderingIndexOverride,
			fulfillmentOrderingIndex:    0,
			disableActiveScheduling:     h.isAutoReturn,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *NoPrivacyWithdrawActionHandler) OnCommitToDB(ctx context.Context) error {
	return nil
}
