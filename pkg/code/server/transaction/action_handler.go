package transaction_v2

import (
	"context"
	"errors"
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

	fulfillmentOrderingIndex uint32
	disableActiveScheduling  bool
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

	// FulfillmentCount returns the total number of fulfillments that
	// will be created for the action.
	FulfillmentCount() int

	// PopulateMetadata populates action metadata into the provided record
	PopulateMetadata(actionRecord *action.Record) error

	// RequiresNonce determines whether a nonce should be acquired for the
	// fulfillment being created. This should be true whenever a virtual
	// instruction needs to be signed by the client.
	RequiresNonce(fulfillmentIndex int) bool

	// GetFulfillmentMetadata gets metadata for the fulfillment being created
	GetFulfillmentMetadata(
		index int,
		nonce *common.Account,
		bh solana.Blockhash,
	) (*newFulfillmentMetadata, error)
}

// UpgradeActionHandler is an interface for upgrading existing actions. It's
// assumed we'll only be upgrading a single fulfillment.
type UpgradeActionHandler interface {
	BaseActionHandler

	// GetFulfillmentBeingUpgraded gets the original fulfillment that's being
	// upgraded.
	GetFulfillmentBeingUpgraded() *fulfillment.Record

	// GetFulfillmentMetadata gets upgraded fulfillment metadata
	GetFulfillmentMetadata(
		nonce *common.Account,
		bh solana.Blockhash,
	) (*newFulfillmentMetadata, error)
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

	timelockAccounts, err := authority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
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

func (h *OpenAccountActionHandler) RequiresNonce(index int) bool {
	return false
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

func (h *OpenAccountActionHandler) OnSaveToDB(ctx context.Context) error {
	err := h.data.SaveTimelock(ctx, h.unsavedTimelockRecord)
	if err != nil {
		return err
	}

	return h.data.CreateAccountInfo(ctx, h.unsavedAccountInfoRecord)
}

type NoPrivacyTransferActionHandler struct {
	source           *common.TimelockAccounts
	destination      *common.Account
	amount           uint64
	isFeePayment     bool // Internally, the mechanics of a fee payment are exactly the same
	isCodeFeePayment bool
}

func NewNoPrivacyTransferActionHandler(protoAction *transactionpb.NoPrivacyTransferAction) (CreateActionHandler, error) {
	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
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

	source, err := sourceAuthority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	var destination *common.Account
	var isCodeFeePayment bool
	if protoAction.Type == transactionpb.FeePaymentAction_CODE {
		destination = feeCollector
		isCodeFeePayment = true
	} else {
		destination, err = common.NewAccountFromProto(protoAction.Destination)
		if err != nil {
			return nil, err
		}
	}

	return &NoPrivacyTransferActionHandler{
		source:           source,
		destination:      destination,
		amount:           protoAction.Amount,
		isFeePayment:     true,
		isCodeFeePayment: isCodeFeePayment,
	}, nil
}

func (h *NoPrivacyTransferActionHandler) FulfillmentCount() int {
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
		var codeDestination *commonpb.SolanaAccountId
		if h.isCodeFeePayment {
			codeDestination = h.destination.ToProto()
		}

		return &transactionpb.ServerParameter{
			Type: &transactionpb.ServerParameter_FeePayment{
				FeePayment: &transactionpb.FeePaymentServerParameter{
					CodeDestination: codeDestination,
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
	source                  *common.TimelockAccounts
	destination             *common.Account
	amount                  uint64
	disableActiveScheduling bool
}

func NewNoPrivacyWithdrawActionHandler(intentRecord *intent.Record, protoAction *transactionpb.NoPrivacyWithdrawAction) (CreateActionHandler, error) {
	var disableActiveScheduling bool

	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		// Technically we should do this for public receives too, but we don't
		// yet have a great way of doing cross intent fulfillment polling hints.
		disableActiveScheduling = true
	}

	sourceAuthority, err := common.NewAccountFromProto(protoAction.Authority)
	if err != nil {
		return nil, err
	}

	source, err := sourceAuthority.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromProto(protoAction.Destination)
	if err != nil {
		return nil, err
	}

	return &NoPrivacyWithdrawActionHandler{
		source:                  source,
		destination:             destination,
		amount:                  protoAction.Amount,
		disableActiveScheduling: disableActiveScheduling,
	}, nil
}

func (h *NoPrivacyWithdrawActionHandler) FulfillmentCount() int {
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

		return &newFulfillmentMetadata{
			requiresClientSignature: true,
			expectedSigner:          h.source.VaultOwner,
			virtualIxnHash:          &virtualIxnHash,

			fulfillmentType:          fulfillment.NoPrivacyWithdraw,
			source:                   h.source.Vault,
			destination:              h.destination,
			fulfillmentOrderingIndex: 0,

			disableActiveScheduling: h.disableActiveScheduling,
		}, nil
	default:
		return nil, errors.New("invalid transaction index")
	}
}

func (h *NoPrivacyWithdrawActionHandler) OnSaveToDB(ctx context.Context) error {
	return nil
}
