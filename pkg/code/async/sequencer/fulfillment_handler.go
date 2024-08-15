package async_sequencer

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	commitment_worker "github.com/code-payments/code-server/pkg/code/async/commitment"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/treasury"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/token"
)

var (
	// Global treasury pool lock
	//
	// todo: Use a distributed lock
	treasuryPoolLock sync.Mutex
)

type FulfillmentHandler interface {
	// CanSubmitToBlockchain determines whether the given fulfillment can be
	// scheduled for submission to the blockchain.
	//
	// Implementations must consider global, account, intent, action and local
	// state relevant to the type of fulfillment being handled to determine if
	// it's safe to schedule.
	//
	// Implementations do not need to validate basic preconditions or basic
	// circuit breaking checks, which is performed by the contextual scheduler.
	CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error)

	// SupportsOnDemandTransactions returns whether a fulfillment type supports
	// on demand transaction creation
	SupportsOnDemandTransactions() bool

	// MakeOnDemandTransaction constructs a transaction at the time of submission
	// to the blockchain. This is an optimization for the nonce pool. Implementations
	// should not modify the provided fulfillment record or selected nonce, but rather
	// use relevant fields to make the corresponding transaction.
	MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error)

	// OnSuccess is a callback function executed on a finalized transaction.
	OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error

	// OnFailure is a callback function executed upon detecting a failed
	// transaction.
	//
	// In general, for automated and manual recovery, the steps should be
	//   1. Ensure the assigned nonce is transitioned back to available
	//      state with the correct blockhash.
	//   2. Update the fulfillment record with a new transaction, plus relevant
	//      metadata (eg. nonce, signature, etc), that does the exact same operation
	//      The fulfillment's state should be pending, so the fulfillment worker can
	//      begin submitting it immediately. The worker does this when recovered = true.
	OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error)

	// IsRevoked checks whether a fulfillment in the unknown state is revoked.
	// It also provides a hint as to whether the nonce was used or not. When in
	// doubt, say no or error out and let a human decide.
	IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error)
}

type InitializeLockedTimelockAccountFulfillmentHandler struct {
	data code_data.Provider
}

func NewInitializeLockedTimelockAccountFulfillmentHandler(data code_data.Provider) FulfillmentHandler {
	return &InitializeLockedTimelockAccountFulfillmentHandler{
		data: data,
	}
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return false, errors.New("invalid fulfillment type")
	}

	accountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	}

	// New primary accounts are scheduled immediately, so the user can receive deposits
	if accountInfoRecord.AccountType == commonpb.AccountType_PRIMARY {
		return true, nil
	}

	// Every other account type needs to be used in a transfer of funds to be opened
	nextScheduledFulfillment, err := h.data.GetNextSchedulableFulfillmentByAddress(ctx, fulfillmentRecord.Source, fulfillmentRecord.IntentOrderingIndex, fulfillmentRecord.ActionId, fulfillmentRecord.FulfillmentOrderingIndex)
	if err != nil {
		return false, err
	}

	switch nextScheduledFulfillment.FulfillmentType {
	case fulfillment.NoPrivacyTransferWithAuthority, fulfillment.NoPrivacyWithdraw, fulfillment.TransferWithCommitment:
		// The account must be the receiver of funds. Obviously it cannot be
		// sending funds if it hasn't been opened yet.
		if nextScheduledFulfillment.Source == fulfillmentRecord.Source || *nextScheduledFulfillment.Destination != fulfillmentRecord.Source {
			return false, errors.New("account being opened is used in an unexpected way")
		}

		return true, nil
	case fulfillment.CloseEmptyTimelockAccount:
		// Technically valid, but we won't open for these cases
		return false, nil
	default:
		// Any other type of fulfillment indicates we're using this account in
		// an unexpected way.
		return false, errors.New("account being opened is used in an unexpected way")
	}
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return true
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return nil, errors.New("invalid fulfillment type")
	}

	timelockRecord, err := h.data.GetTimelockByVault(ctx, fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}

	authorityAccount, err := common.NewAccountFromPublicKeyString(timelockRecord.VaultOwner)
	if err != nil {
		return nil, err
	}

	timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
	if err != nil {
		return nil, err
	}

	memory, accountIndex, err := reserveVmMemory(ctx, h.data, common.CodeVmAccount, cvm.VirtualAccountTypeTimelock, timelockAccounts.Vault)
	if err != nil {
		return nil, err
	}

	txn, err := transaction_util.MakeOpenAccountTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,

		memory,
		accountIndex,

		timelockAccounts,
	)
	if err != nil {
		return nil, err
	}

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return errors.New("invalid fulfillment type")
	}

	return markTimelockLocked(ctx, h.data, fulfillmentRecord.Source, txnRecord.Slot)
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return false, errors.New("invalid fulfillment type")
	}

	// Fulfillment record needs to be scheduled with a new transaction.
	//
	// todo: Implement auto-recovery
	return false, nil
}

func (h *InitializeLockedTimelockAccountFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type NoPrivacyTransferWithAuthorityFulfillmentHandler struct {
	data code_data.Provider
}

func NewNoPrivacyTransferWithAuthorityFulfillmentHandler(data code_data.Provider) FulfillmentHandler {
	return &NoPrivacyTransferWithAuthorityFulfillmentHandler{
		data: data,
	}
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// The destination user account might be a Code account or external wallet, so we
	// must validate it exists on the blockchain prior to send funds to it.
	isDestinationAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, *fulfillmentRecord.Destination)
	if err != nil {
		return false, err
	} else if !isDestinationAccountCreated {
		return false, nil
	}

	// Check whether there's an earlier fulfillment that should be scheduled first
	// where the source user account is the destination. This fulfillment might depend
	// on the receipt of some funds.
	earliestFulfillmentForSourceAsDestination, err := h.data.GetFirstSchedulableFulfillmentByAddressAsDestination(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillmentForSourceAsDestination != nil && earliestFulfillmentForSourceAsDestination.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	return true, nil
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return errors.New("invalid fulfillment type")
	}

	return savePaymentRecord(ctx, h.data, fulfillmentRecord, txnRecord)
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	// This is bad, we need to make the user whole
	return false, nil
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return false
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	return nil, errors.New("not supported")
}

type NoPrivacyWithdrawFulfillmentHandler struct {
	data code_data.Provider
}

func NewNoPrivacyWithdrawFulfillmentHandler(data code_data.Provider) FulfillmentHandler {
	return &NoPrivacyWithdrawFulfillmentHandler{
		data: data,
	}
}

func (h *NoPrivacyWithdrawFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return false, errors.New("invalid fulfillment type")
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// The destination user account might be a Code account or external wallet, so we
	// must validate it exists on the blockchain prior to send funds to it.
	isDestinationAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, *fulfillmentRecord.Destination)
	if err != nil {
		return false, err
	} else if !isDestinationAccountCreated {
		return false, nil
	}

	// todo: We can have single "AsSourceOrDestination" query

	// Check whether there's an earlier fulfillment that should be scheduled first
	// where the source user account is the source. The account will be closed, so
	// any prior transfers must be completed.
	earliestFulfillment, err := h.data.GetFirstSchedulableFulfillmentByAddressAsSource(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillment != nil && earliestFulfillment.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	// Check whether there's an earlier fulfillment that should be scheduled first
	// where the source user account is the destination. This fulfillment might depend
	// on the receipt of some funds.
	earliestFulfillment, err = h.data.GetFirstSchedulableFulfillmentByAddressAsDestination(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillment != nil && earliestFulfillment.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	return true, nil
}

func (h *NoPrivacyWithdrawFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return false
}

func (h *NoPrivacyWithdrawFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	return nil, errors.New("not supported")
}

func (h *NoPrivacyWithdrawFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return errors.New("invalid fulfillment type")
	}

	err := savePaymentRecord(ctx, h.data, fulfillmentRecord, txnRecord)
	if err != nil {
		return err
	}

	return onVirtualAccountDeleted(ctx, h.data, fulfillmentRecord.Source)
}

func (h *NoPrivacyWithdrawFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return false, errors.New("invalid fulfillment type")
	}

	// This is bad, we need to make the user whole
	return false, nil
}

func (h *NoPrivacyWithdrawFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type TemporaryPrivacyTransferWithAuthorityFulfillmentHandler struct {
	conf *conf
	data code_data.Provider
}

func NewTemporaryPrivacyTransferWithAuthorityFulfillmentHandler(data code_data.Provider, configProvider ConfigProvider) FulfillmentHandler {
	return &TemporaryPrivacyTransferWithAuthorityFulfillmentHandler{
		conf: configProvider(),
		data: data,
	}
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TemporaryPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	commitmentRecord, err := h.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return false, err
	}

	// Sanity check that we haven't upgraded this private transfer
	if commitmentRecord.RepaymentDivertedTo != nil {
		return false, nil
	}

	// The commitment must be opened before we can send funds to it
	if commitmentRecord.State != commitment.StateOpen {
		return false, nil
	}

	// Check the privacy upgrade deadline, which is one of many factors as to
	// why we may have opened the commitment. We need to ensure the deadline
	// is hit before proceeding.
	privacyUpgradeDeadline, err := commitment_worker.GetDeadlineToUpgradePrivacy(ctx, h.data, commitmentRecord)
	if err == commitment_worker.ErrNoPrivacyUpgradeDeadline {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// The deadline to upgrade privacy hasn't been met, so don't schedule it
	if privacyUpgradeDeadline.After(time.Now()) {
		return false, nil
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// Check whether there's an earlier fulfillment that should be scheduled first
	// where the source user account is the destination. This fulfillment might depend
	// on the receipt of some funds.
	earliestFulfillmentForSourceAsDestination, err := h.data.GetFirstSchedulableFulfillmentByAddressAsDestination(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillmentForSourceAsDestination != nil && earliestFulfillmentForSourceAsDestination.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	recordTemporaryPrivateTransferScheduledEvent(ctx, fulfillmentRecord)
	return true, nil
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return false
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	return nil, errors.New("not supported")
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.TemporaryPrivacyTransferWithAuthority {
		return errors.New("invalid fulfillment type")
	}

	return savePaymentRecord(ctx, h.data, fulfillmentRecord, txnRecord)
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TemporaryPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	// This is bad. The treasury pool cannot be refunded
	return false, nil
}

func (h *TemporaryPrivacyTransferWithAuthorityFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TemporaryPrivacyTransferWithAuthority {
		return false, false, errors.New("invalid fulfillment type")
	}

	count, err := h.data.GetFulfillmentCountByTypeActionAndState(
		ctx,
		fulfillmentRecord.Intent,
		fulfillmentRecord.ActionId,
		fulfillment.PermanentPrivacyTransferWithAuthority,
		fulfillment.StateConfirmed,
	)
	if err != nil {
		return false, false, err
	}

	// Temporary private transfer is revoked when the corresponding permanent
	// private transfer in the same action is confirmed.
	if count == 0 {
		return false, false, nil
	}

	nonceRecord, err := h.data.GetNonce(ctx, *fulfillmentRecord.Nonce)
	if err != nil {
		return false, false, err
	}

	// Sanity check because this is dangerous since the blockhash would never be
	// progressed and we'd be using a stale one on the next transaction. In an
	// ideal world, this points to the upgraded fulfillment or nothing at all.
	if nonceRecord.Signature == *fulfillmentRecord.Signature {
		return false, false, errors.New("too dangerous to revoke fulfillment")
	}

	return true, true, nil
}

type PermanentPrivacyTransferWithAuthorityFulfillmentHandler struct {
	conf *conf
	data code_data.Provider
}

func NewPermanentPrivacyTransferWithAuthorityFulfillmentHandler(data code_data.Provider, configProvider ConfigProvider) FulfillmentHandler {
	return &PermanentPrivacyTransferWithAuthorityFulfillmentHandler{
		conf: configProvider(),
		data: data,
	}
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.PermanentPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	oldCommitmentRecord, err := h.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return false, err
	}

	// The old commitment record must be marked as diverting funds to the new
	// intended commitment before proceeding.
	if oldCommitmentRecord.RepaymentDivertedTo == nil {
		return false, nil
	}

	newCommitmentRecord, err := h.data.GetCommitmentByAddress(ctx, *oldCommitmentRecord.RepaymentDivertedTo)
	if err != nil {
		return false, err
	}

	// The commitment vault must be opened before we can send funds to it
	if newCommitmentRecord.State != commitment.StateOpen {
		return false, nil
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// Check whether there's an earlier fulfillment that should be scheduled first
	// where the source user account is the destination. This fulfillment might depend
	// on the receipt of some funds.
	earliestFulfillmentForSourceAsDestination, err := h.data.GetFirstSchedulableFulfillmentByAddressAsDestination(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillmentForSourceAsDestination != nil && earliestFulfillmentForSourceAsDestination.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	return true, nil
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return false
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	return nil, errors.New("not supported")
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.PermanentPrivacyTransferWithAuthority {
		return errors.New("invalid fulfillment type")
	}

	err := savePaymentRecord(ctx, h.data, fulfillmentRecord, txnRecord)
	if err != nil {
		return err
	}

	// Wake up the temporary privacy transaction so we can process it to a revoked state
	temporaryTransferFulfillment, err := h.data.GetAllFulfillmentsByTypeAndAction(ctx, fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return err
	} else if err == nil {
		return markFulfillmentAsActivelyScheduled(ctx, h.data, temporaryTransferFulfillment[0])
	}
	return nil
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.PermanentPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	// This is bad. The treasury pool cannot be refunded
	return false, nil
}

func (h *PermanentPrivacyTransferWithAuthorityFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.PermanentPrivacyTransferWithAuthority {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type TransferWithCommitmentFulfillmentHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewTransferWithCommitmentFulfillmentHandler(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) FulfillmentHandler {
	return &TransferWithCommitmentFulfillmentHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

func (h *TransferWithCommitmentFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
		return false, errors.New("invalid fulfillment type")
	}

	// Ensure the commitment record exists and it's in a valid initial state.
	commitmentRecord, err := h.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return false, err
	} else if commitmentRecord.State != commitment.StateUnknown && commitmentRecord.State != commitment.StatePayingDestination {
		return false, errors.New("commitment in unexpected state")
	}

	// The destination account is a Code account, so we must validate it exists
	// on the blockchain prior to sending funds to it.
	isDestinationAccountCreated, err := isTokenAccountOnBlockchain(ctx, h.data, *fulfillmentRecord.Destination)
	if err != nil {
		return false, err
	} else if !isDestinationAccountCreated {
		return false, nil
	}

	// If our funds aren't already reserved for use with the treasury pool, then we
	// need to check if there's sufficient funding to pay the destination.
	if commitmentRecord.State != commitment.StatePayingDestination {
		// No need to include the state transition in the lock yet, since we transition
		// the commitment account to a state where the funds will be reserved. If the DB
		// has a failure, we'll just retry scheduling and it will go through the next
		// time.
		treasuryPoolLock.Lock()
		defer treasuryPoolLock.Unlock()

		poolRecord, err := h.data.GetTreasuryPoolByAddress(ctx, commitmentRecord.Pool)
		if err != nil {
			return false, err
		}

		totalAvailableTreasuryPoolFunds, usedTreasuryPoolFunds, err := estimateTreasuryPoolFundingLevels(ctx, h.data, poolRecord)
		if err != nil {
			return false, err
		}

		// The treasury pool's funds are used entirely
		if usedTreasuryPoolFunds >= totalAvailableTreasuryPoolFunds {
			return false, nil
		}

		// The treasury pool doesn't have sufficient funds to transfer to the destination
		// account.
		remainingTreasuryPoolFunds := totalAvailableTreasuryPoolFunds - usedTreasuryPoolFunds
		if remainingTreasuryPoolFunds < commitmentRecord.Amount {
			return false, nil
		}

		// Mark the commitment as paying the destination, so we can track the funds we're
		// going to be using from the treasury pool.
		err = markCommitmentPayingDestination(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func (h *TransferWithCommitmentFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return true
}

func (h *TransferWithCommitmentFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	commitmentRecord, err := h.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return nil, err
	}

	if commitmentRecord.State != commitment.StatePayingDestination {
		return nil, errors.New("commitment in unexpected state")
	}

	timelockRecord, err := h.data.GetTimelockByVault(ctx, commitmentRecord.Destination)
	if err != nil {
		return nil, err
	}
	destinationTimelockOwner, err := common.NewAccountFromPublicKeyString(timelockRecord.VaultOwner)
	if err != nil {
		return nil, err
	}

	treasuryPool, err := common.NewAccountFromPublicKeyString(commitmentRecord.Pool)
	if err != nil {
		return nil, err
	}

	treasuryPoolVault, err := common.NewAccountFromPublicKeyString(fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}

	destination, err := common.NewAccountFromPublicKeyString(commitmentRecord.Destination)
	if err != nil {
		return nil, err
	}

	commitment, err := common.NewAccountFromPublicKeyString(commitmentRecord.Address)
	if err != nil {
		return nil, err
	}

	transcript, err := hex.DecodeString(commitmentRecord.Transcript)
	if err != nil {
		return nil, err
	}

	recentRoot, err := hex.DecodeString(commitmentRecord.RecentRoot)
	if err != nil {
		return nil, err
	}

	_, timelockAccountMemory, timelockAccountIndex, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, destinationTimelockOwner)
	if err != nil {
		return nil, err
	}

	relayMemory, relayAccountIndex, err := reserveVmMemory(ctx, h.data, common.CodeVmAccount, cvm.VirtualAccountTypeRelay, commitment)
	if err != nil {
		return nil, err
	}

	// todo: support external transfers
	txn, err := transaction_util.MakeInternalTreasuryAdvanceTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,

		common.CodeVmAccount,
		timelockAccountMemory,
		timelockAccountIndex,
		relayMemory,
		relayAccountIndex,

		treasuryPool,
		treasuryPoolVault,
		destination,
		commitment,
		uint32(commitmentRecord.Amount), // todo: assumes amount never overflows uint32
		transcript,
		recentRoot,
	)
	if err != nil {
		return nil, err
	}

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (h *TransferWithCommitmentFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
		return errors.New("invalid fulfillment type")
	}

	err := savePaymentRecord(ctx, h.data, fulfillmentRecord, txnRecord)
	if err != nil {
		return err
	}

	return markCommitmentOpen(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
}

func (h *TransferWithCommitmentFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
		return false, errors.New("invalid fulfillment type")
	}

	// Fulfillment record needs to be scheduled with a new transaction
	//
	// todo: Implement auto-recovery
	return false, nil
}

func (h *TransferWithCommitmentFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.TransferWithCommitment {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type CloseEmptyTimelockAccountFulfillmentHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewCloseEmptyTimelockAccountFulfillmentHandler(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) FulfillmentHandler {
	return &CloseEmptyTimelockAccountFulfillmentHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return false, errors.New("invalid fulfillment type")
	}

	// todo: We can have single "AsSourceOrDestination" query

	// The source account is a user account, so check that there are no other
	// fulfillments where it's used as a source account.
	earliestFulfillment, err := h.data.GetFirstSchedulableFulfillmentByAddressAsSource(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillment != nil && earliestFulfillment.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	// The source account is a user account, so check that there are no other
	// fulfillments where it's used as a destination account.
	earliestFulfillment, err = h.data.GetFirstSchedulableFulfillmentByAddressAsDestination(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestFulfillment != nil && earliestFulfillment.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	return true, nil
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return true
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return nil, errors.New("invalid fulfillment type")
	}

	timelockVault, err := common.NewAccountFromPublicKeyString(fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}
	timelockRecord, err := h.data.GetTimelockByVault(ctx, timelockVault.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}
	timelockOwner, err := common.NewAccountFromPublicKeyString(timelockRecord.VaultOwner)
	if err != nil {
		return nil, err
	}

	virtualAccountState, memory, index, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, timelockOwner)
	if err != nil {
		return nil, err
	}

	if virtualAccountState.Balance != 0 {
		return nil, errors.New("stale timelock account state")
	}

	storage, err := reserveVmStorage(ctx, h.data, common.CodeVmAccount, storage.PurposeDeletion, timelockVault)
	if err != nil {
		return nil, err
	}

	txn, err := transaction_util.MakeCompressAccountTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,

		common.CodeVmAccount,
		memory,
		index,
		storage,
		virtualAccountState.Marshal(),
	)
	if err != nil {
		return nil, err
	}

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return errors.New("invalid fulfillment type")
	}

	return onVirtualAccountDeleted(ctx, h.data, fulfillmentRecord.Source)
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return false, errors.New("invalid fulfillment type")
	}

	// Fulfillment record needs to be scheduled with a new transaction, which may
	// or may not need to be signed by the client. It all depends on whether there
	// is dust in the account.
	//
	// todo: Implement auto-recovery when we know the account is empty
	// todo: Do "something" to indicate the client needs to resign a new transaction
	return false, nil
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type SaveRecentRootFulfillmentHandler struct {
	data code_data.Provider
}

func NewSaveRecentRootFulfillmentHandler(data code_data.Provider) FulfillmentHandler {
	return &SaveRecentRootFulfillmentHandler{
		data: data,
	}
}

// Assumption: Saving a recent root is pre-sorted to the back of the line
func (h *SaveRecentRootFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.SaveRecentRoot {
		return false, errors.New("invalid fulfillment type")
	}

	// Ensure that any prior TransferWithCommitment fulfillments are played out
	// before saving the recent root. This will ensure the scheduler itself won't
	// go too fast and risk having outdated recent roots. It's not perfect, and
	// it's still up to the process making SaveRecentRoot intents to determine
	// when it's safe to do so.
	//
	// Note: The treasury is only the source of funds for TransferWithCommitment transactions.
	earliestTransferWithCommitment, err := h.data.GetFirstSchedulableFulfillmentByAddressAsSource(ctx, fulfillmentRecord.Source)
	if err != nil && err != fulfillment.ErrFulfillmentNotFound {
		return false, err
	}
	if earliestTransferWithCommitment != nil && earliestTransferWithCommitment.ScheduledBefore(fulfillmentRecord) {
		return false, nil
	}

	return true, nil
}

func (h *SaveRecentRootFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return false
}

func (h *SaveRecentRootFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	return nil, errors.New("not supported")
}

func (h *SaveRecentRootFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.SaveRecentRoot {
		return errors.New("invalid fulfillment type")
	}

	return nil
}

func (h *SaveRecentRootFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.SaveRecentRoot {
		return false, errors.New("invalid fulfillment type")
	}

	// Fulfillment record needs to be scheduled with a new transaction
	//
	// todo: Implement auto-recovery
	return false, nil
}

func (h *SaveRecentRootFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.SaveRecentRoot {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

type CloseCommitmentFulfillmentHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewCloseCommitmentFulfillmentHandler(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) FulfillmentHandler {
	return &CloseCommitmentFulfillmentHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

// todo: New commitment closing flow not implemented yet
func (h *CloseCommitmentFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	commitmentRecord, err := h.data.GetCommitmentByAction(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return false, err
	}

	// Commitment worker guarantees all cheques have been cashed
	return commitmentRecord.State == commitment.StateClosing, nil
}

func (h *CloseCommitmentFulfillmentHandler) SupportsOnDemandTransactions() bool {
	return true
}

func (h *CloseCommitmentFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.SelectedNonce) (*solana.Transaction, error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseCommitment {
		return nil, errors.New("invalid fulfillment type")
	}

	relay, err := common.NewAccountFromPublicKeyString(fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}

	virtualAccountState, memory, index, err := getVirtualRelayAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, relay)
	if err != nil {
		return nil, err
	}

	storage, err := reserveVmStorage(ctx, h.data, common.CodeVmAccount, storage.PurposeDeletion, relay)
	if err != nil {
		return nil, err
	}

	txn, err := transaction_util.MakeCompressAccountTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,

		common.CodeVmAccount,
		memory,
		index,
		storage,
		virtualAccountState.Marshal(),
	)
	if err != nil {
		return nil, err
	}

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (h *CloseCommitmentFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseCommitment {
		return errors.New("invalid fulfillment type")
	}

	err := markCommitmentClosed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return err
	}

	return onVirtualAccountDeleted(ctx, h.data, fulfillmentRecord.Source)
}

func (h *CloseCommitmentFulfillmentHandler) OnFailure(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) (recovered bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseCommitment {
		return false, errors.New("invalid fulfillment type")
	}

	// Let it fail. More than likely we have a bug. In theory, we could try implementing
	// auto-recovery.
	return false, nil
}

func (h *CloseCommitmentFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseCommitment {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

func isTokenAccountOnBlockchain(ctx context.Context, data code_data.Provider, address string) (bool, error) {
	// Optimization for external accounts managed by Code
	switch address {
	case "Ad4gWGCB94PsA4cP2jqSjfg7eTi4aVkrEdXXhNivT8nW": // Fee collector
		return true, nil
	}

	// Try our cache of Code timelock accounts
	timelockRecord, err := data.GetTimelockByVault(ctx, address)
	if err == timelock.ErrTimelockNotFound {
		// Likely not a Code timelock account, so defer to the blockchain
		_, err := data.GetBlockchainTokenAccountInfo(ctx, address, solana.CommitmentFinalized)
		if err == solana.ErrNoAccountInfo || err == token.ErrAccountNotFound {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return true, nil
	} else if err != nil {
		return false, err
	}

	existsOnBlockchain := timelockRecord.ExistsOnBlockchain()

	// We've detected the use of an account that's not on the blockchain, so
	// best-effort kick off active scheduling for the InitializeLockedTimelockAccount
	// fulfillment.
	if !existsOnBlockchain {
		// Initializing an account is always the first thing scheduled
		initializeFulfillmentRecord, err := data.GetFirstSchedulableFulfillmentByAddressAsSource(ctx, address)
		if err != nil {
			return existsOnBlockchain, nil
		}

		if initializeFulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
			return existsOnBlockchain, nil
		}

		markFulfillmentAsActivelyScheduled(ctx, data, initializeFulfillmentRecord)
	}

	return existsOnBlockchain, nil
}

func estimateTreasuryPoolFundingLevels(ctx context.Context, data code_data.Provider, record *treasury.Record) (total uint64, used uint64, err error) {
	total, err = data.GetTotalAvailableTreasuryPoolFunds(ctx, record.Vault)
	if err != nil {
		return 0, 0, err
	}

	used, err = data.GetUsedTreasuryPoolDeficitFromCommitments(ctx, record.Address)
	if err != nil {
		return 0, 0, err
	}

	return total, used, nil
}

// todo: simplify initialization of fulfillment handlers across service and contextual scheduler
func getFulfillmentHandlers(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, configProvider ConfigProvider) map[fulfillment.Type]FulfillmentHandler {
	handlersByType := make(map[fulfillment.Type]FulfillmentHandler)
	handlersByType[fulfillment.InitializeLockedTimelockAccount] = NewInitializeLockedTimelockAccountFulfillmentHandler(data)
	handlersByType[fulfillment.NoPrivacyTransferWithAuthority] = NewNoPrivacyTransferWithAuthorityFulfillmentHandler(data)
	handlersByType[fulfillment.NoPrivacyWithdraw] = NewNoPrivacyWithdrawFulfillmentHandler(data)
	handlersByType[fulfillment.TemporaryPrivacyTransferWithAuthority] = NewTemporaryPrivacyTransferWithAuthorityFulfillmentHandler(data, configProvider)
	handlersByType[fulfillment.PermanentPrivacyTransferWithAuthority] = NewPermanentPrivacyTransferWithAuthorityFulfillmentHandler(data, configProvider)
	handlersByType[fulfillment.TransferWithCommitment] = NewTransferWithCommitmentFulfillmentHandler(data, vmIndexerClient)
	handlersByType[fulfillment.CloseEmptyTimelockAccount] = NewCloseEmptyTimelockAccountFulfillmentHandler(data, vmIndexerClient)
	handlersByType[fulfillment.SaveRecentRoot] = NewSaveRecentRootFulfillmentHandler(data)
	handlersByType[fulfillment.CloseCommitment] = NewCloseCommitmentFulfillmentHandler(data, vmIndexerClient)
	return handlersByType
}
