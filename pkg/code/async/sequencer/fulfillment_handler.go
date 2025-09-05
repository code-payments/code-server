package async_sequencer

import (
	"context"
	"errors"

	"github.com/mr-tron/base58"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/token"
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
	//
	// Note: This is also being abused for an initial version of packing virtual
	// instructions 1:1 into a Solana transaction. A new flow/strategy will be
	// needed when we require more efficient packing.
	SupportsOnDemandTransactions() bool

	// MakeOnDemandTransaction constructs a transaction at the time of submission
	// to the blockchain. This is an optimization for the nonce pool. Implementations
	// should not modify the provided fulfillment record or selected nonce, but rather
	// use relevant fields to make the corresponding transaction.
	MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.Nonce) (*solana.Transaction, error)

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
	if err == fulfillment.ErrFulfillmentNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}

	switch nextScheduledFulfillment.FulfillmentType {
	case fulfillment.NoPrivacyTransferWithAuthority, fulfillment.NoPrivacyWithdraw:
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

func (h *InitializeLockedTimelockAccountFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.Nonce) (*solana.Transaction, error) {
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

	timelockAccounts, err := authorityAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
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
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewNoPrivacyTransferWithAuthorityFulfillmentHandler(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) FulfillmentHandler {
	return &NoPrivacyTransferWithAuthorityFulfillmentHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return false, errors.New("invalid fulfillment type")
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isAccountInitialized(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// The destination user account might be a Code account or external wallet, so we
	// must validate it exists on the blockchain prior to sending funds to it, or if we'll
	// be creating it at time of send.
	destinationAccount, err := common.NewAccountFromPublicKeyString(*fulfillmentRecord.Destination)
	if err != nil {
		return false, err
	}
	isInternalTransfer, err := isInternalVmTransfer(ctx, h.data, destinationAccount)
	if err != nil {
		return false, err
	}
	hasCreateOnSendFee, err := h.data.HasFeeAction(ctx, fulfillmentRecord.Intent, transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL)
	if err != nil {
		return false, err
	}
	if isInternalTransfer || !hasCreateOnSendFee {
		isDestinationAccountCreated, err := isAccountInitialized(ctx, h.data, *fulfillmentRecord.Destination)
		if err != nil {
			return false, err
		} else if !isDestinationAccountCreated {
			return false, nil
		}
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

	return nil
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
	return true
}

func (h *NoPrivacyTransferWithAuthorityFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.Nonce) (*solana.Transaction, error) {
	actionRecord, err := h.data.GetActionById(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return nil, err
	}

	virtualSignatureBytes, err := base58.Decode(*fulfillmentRecord.VirtualSignature)
	if err != nil {
		return nil, err
	}

	virtualNonce, err := common.NewAccountFromPublicKeyString(*fulfillmentRecord.VirtualNonce)
	if err != nil {
		return nil, err
	}

	sourceVault, err := common.NewAccountFromPublicKeyString(fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}

	sourceAccountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, sourceVault.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceAuthority, err := common.NewAccountFromPublicKeyString(sourceAccountInfoRecord.AuthorityAccount)
	if err != nil {
		return nil, err
	}

	destinationTokenAccount, err := common.NewAccountFromPublicKeyString(*fulfillmentRecord.Destination)
	if err != nil {
		return nil, err
	}

	_, nonceMemory, nonceIndex, err := getVirtualDurableNonceAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, virtualNonce)
	if err != nil {
		return nil, err
	}

	_, sourceMemory, sourceIndex, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, sourceAuthority)
	if err != nil {
		return nil, err
	}

	isInternal, err := isInternalVmTransfer(ctx, h.data, destinationTokenAccount)
	if err != nil {
		return nil, err
	}

	var txn solana.Transaction
	var makeTxnErr error
	if isInternal {
		destinationAccountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, destinationTokenAccount.PublicKey().ToBase58())
		if err != nil {
			return nil, err
		}

		destinationAuthority, err := common.NewAccountFromPublicKeyString(destinationAccountInfoRecord.AuthorityAccount)
		if err != nil {
			return nil, err
		}

		_, destinationMemory, destinationIndex, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, destinationAuthority)
		if err != nil {
			return nil, err
		}

		txn, makeTxnErr = transaction_util.MakeInternalTransferWithAuthorityTransaction(
			selectedNonce.Account,
			selectedNonce.Blockhash,

			solana.Signature(virtualSignatureBytes),

			common.CodeVmAccount,
			nonceMemory,
			nonceIndex,
			sourceMemory,
			sourceIndex,
			destinationMemory,
			destinationIndex,

			*actionRecord.Quantity,
		)
	} else {
		isFeePayment := actionRecord.FeeType != nil

		var isCreateOnSend bool
		// The Fee payment can be an external transfer, but we know the account
		// already exists and doesn't need an idempotent create instruction
		if !isFeePayment {
			isCreateOnSend, err = h.data.HasFeeAction(ctx, fulfillmentRecord.Intent, transactionpb.FeePaymentAction_CREATE_ON_SEND_WITHDRAWAL)
			if err != nil {
				return &solana.Transaction{}, err
			}
		}

		var destinationOwnerAccount *common.Account
		if isCreateOnSend {
			intentRecord, err := h.data.GetIntent(ctx, fulfillmentRecord.Intent)
			if err != nil {
				return nil, err
			}

			destinationOwnerAccount, err = common.NewAccountFromPublicKeyString(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount)
			if err != nil {
				return nil, err
			}
		}

		txn, makeTxnErr = transaction_util.MakeExternalTransferWithAuthorityTransaction(
			selectedNonce.Account,
			selectedNonce.Blockhash,

			solana.Signature(virtualSignatureBytes),

			common.CodeVmAccount,
			common.CodeVmOmnibusAccount,

			nonceMemory,
			nonceIndex,
			sourceMemory,
			sourceIndex,

			isCreateOnSend,
			destinationOwnerAccount,
			destinationTokenAccount,
			common.CoreMintAccount,

			*actionRecord.Quantity,
		)
	}
	if makeTxnErr != nil {
		return nil, makeTxnErr
	}
	return &txn, nil
}

type NoPrivacyWithdrawFulfillmentHandler struct {
	data            code_data.Provider
	vmIndexerClient indexerpb.IndexerClient
}

func NewNoPrivacyWithdrawFulfillmentHandler(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) FulfillmentHandler {
	return &NoPrivacyWithdrawFulfillmentHandler{
		data:            data,
		vmIndexerClient: vmIndexerClient,
	}
}

func (h *NoPrivacyWithdrawFulfillmentHandler) CanSubmitToBlockchain(ctx context.Context, fulfillmentRecord *fulfillment.Record) (scheduled bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return false, errors.New("invalid fulfillment type")
	}

	// The source user account is a Code account, so we must validate it exists on
	// the blockchain prior to sending funds from it.
	isSourceAccountCreated, err := isAccountInitialized(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return false, err
	} else if !isSourceAccountCreated {
		return false, nil
	}

	// The destination user account might be a Code account or external wallet, so we
	// must validate it exists on the blockchain prior to send funds to it.
	isDestinationAccountCreated, err := isAccountInitialized(ctx, h.data, *fulfillmentRecord.Destination)
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
	return true
}

func (h *NoPrivacyWithdrawFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.Nonce) (*solana.Transaction, error) {
	virtualSignatureBytes, err := base58.Decode(*fulfillmentRecord.VirtualSignature)
	if err != nil {
		return nil, err
	}

	virtualNonce, err := common.NewAccountFromPublicKeyString(*fulfillmentRecord.VirtualNonce)
	if err != nil {
		return nil, err
	}

	sourceVault, err := common.NewAccountFromPublicKeyString(fulfillmentRecord.Source)
	if err != nil {
		return nil, err
	}

	sourceAccountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, sourceVault.PublicKey().ToBase58())
	if err != nil {
		return nil, err
	}

	sourceAuthority, err := common.NewAccountFromPublicKeyString(sourceAccountInfoRecord.AuthorityAccount)
	if err != nil {
		return nil, err
	}

	destinationTokenAccount, err := common.NewAccountFromPublicKeyString(*fulfillmentRecord.Destination)
	if err != nil {
		return nil, err
	}

	_, nonceMemory, nonceIndex, err := getVirtualDurableNonceAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, virtualNonce)
	if err != nil {
		return nil, err
	}

	_, sourceMemory, sourceIndex, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, sourceAuthority)
	if err != nil {
		return nil, err
	}

	isInternal, err := isInternalVmTransfer(ctx, h.data, destinationTokenAccount)
	if err != nil {
		return nil, err
	}

	var txn solana.Transaction
	var makeTxnErr error
	if isInternal {
		destinationAccountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, destinationTokenAccount.PublicKey().ToBase58())
		if err != nil {
			return nil, err
		}

		destinationAuthority, err := common.NewAccountFromPublicKeyString(destinationAccountInfoRecord.AuthorityAccount)
		if err != nil {
			return nil, err
		}

		_, destinationMemory, destinationIndex, err := getVirtualTimelockAccountStateInMemory(ctx, h.vmIndexerClient, common.CodeVmAccount, destinationAuthority)
		if err != nil {
			return nil, err
		}

		txn, makeTxnErr = transaction_util.MakeInternalWithdrawTransaction(
			selectedNonce.Account,
			selectedNonce.Blockhash,

			solana.Signature(virtualSignatureBytes),

			common.CodeVmAccount,
			nonceMemory,
			nonceIndex,
			sourceMemory,
			sourceIndex,
			destinationMemory,
			destinationIndex,
		)
	} else {
		txn, makeTxnErr = transaction_util.MakeExternalWithdrawTransaction(
			selectedNonce.Account,
			selectedNonce.Blockhash,

			solana.Signature(virtualSignatureBytes),

			common.CodeVmAccount,
			common.CodeVmOmnibusAccount,

			nonceMemory,
			nonceIndex,
			sourceMemory,
			sourceIndex,

			destinationTokenAccount,
		)
	}
	if makeTxnErr != nil {
		return nil, makeTxnErr
	}
	return &txn, nil
}

func (h *NoPrivacyWithdrawFulfillmentHandler) OnSuccess(ctx context.Context, fulfillmentRecord *fulfillment.Record, txnRecord *transaction.Record) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return errors.New("invalid fulfillment type")
	}

	err := onVirtualAccountDeleted(ctx, h.data, fulfillmentRecord.Source)
	if err != nil {
		return err
	}

	err = markTimelockClosed(ctx, h.data, fulfillmentRecord.Source, txnRecord.Slot)
	if err != nil {
		return err
	}

	return maybeCleanupAutoReturnAction(ctx, h.data, fulfillmentRecord.Source)
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

	// Auto-return actions might be revoked, check the action record that would've
	// been updated by the gift card return worker.
	actionRecord, err := h.data.GetActionById(ctx, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	if err != nil {
		return false, false, err
	}

	return actionRecord.State == action.StateRevoked, false, nil
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

func (h *CloseEmptyTimelockAccountFulfillmentHandler) MakeOnDemandTransaction(ctx context.Context, fulfillmentRecord *fulfillment.Record, selectedNonce *transaction_util.Nonce) (*solana.Transaction, error) {
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
	return false, nil
}

func (h *CloseEmptyTimelockAccountFulfillmentHandler) IsRevoked(ctx context.Context, fulfillmentRecord *fulfillment.Record) (revoked bool, nonceUsed bool, err error) {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return false, false, errors.New("invalid fulfillment type")
	}

	return false, false, nil
}

func isAccountInitialized(ctx context.Context, data code_data.Provider, address string) (bool, error) {
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

// todo: simplify initialization of fulfillment handlers across service and contextual scheduler
func getFulfillmentHandlers(data code_data.Provider, vmIndexerClient indexerpb.IndexerClient) map[fulfillment.Type]FulfillmentHandler {
	handlersByType := make(map[fulfillment.Type]FulfillmentHandler)
	handlersByType[fulfillment.InitializeLockedTimelockAccount] = NewInitializeLockedTimelockAccountFulfillmentHandler(data)
	handlersByType[fulfillment.NoPrivacyTransferWithAuthority] = NewNoPrivacyTransferWithAuthorityFulfillmentHandler(data, vmIndexerClient)
	handlersByType[fulfillment.NoPrivacyWithdraw] = NewNoPrivacyWithdrawFulfillmentHandler(data, vmIndexerClient)
	handlersByType[fulfillment.CloseEmptyTimelockAccount] = NewCloseEmptyTimelockAccountFulfillmentHandler(data, vmIndexerClient)
	return handlersByType
}
