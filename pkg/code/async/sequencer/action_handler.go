package async_sequencer

import (
	"context"
	"errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

var (
	ErrInvalidActionStateTransition = errors.New("invalid action state transition")
)

type ActionHandler interface {
	// Note: New fulfillment states are not recorded until the very end of a worker
	//       run, which is why we need to pass it into this function.
	OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error
}

type OpenAccountActionHandler struct {
	data code_data.Provider
}

func NewOpenAccountActionHandler(data code_data.Provider) ActionHandler {
	return &OpenAccountActionHandler{
		data: data,
	}
}

func (h *OpenAccountActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.InitializeLockedTimelockAccount {
		return errors.New("unexpected fulfillment type")
	}

	if newState == fulfillment.StateConfirmed {
		return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	if newState == fulfillment.StateFailed {
		return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	return nil
}

type CloseEmptyAccountActionHandler struct {
	data code_data.Provider
}

func NewCloseEmptyAccountActionHandler(data code_data.Provider) ActionHandler {
	return &CloseEmptyAccountActionHandler{
		data: data,
	}
}

func (h *CloseEmptyAccountActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseEmptyTimelockAccount {
		return errors.New("unexpected fulfillment type")
	}

	if newState == fulfillment.StateConfirmed {
		return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	if newState == fulfillment.StateFailed {
		return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	return nil
}

type NoPrivacyTransferActionHandler struct {
	data code_data.Provider
}

func NewNoPrivacyTransferActionHandler(data code_data.Provider) ActionHandler {
	return &NoPrivacyTransferActionHandler{
		data: data,
	}
}

func (h *NoPrivacyTransferActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyTransferWithAuthority {
		return errors.New("unexpected fulfillment type")
	}

	if newState == fulfillment.StateConfirmed {
		return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	if newState == fulfillment.StateFailed {
		return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	return nil
}

type NoPrivacyWithdrawActionHandler struct {
	data code_data.Provider
}

func NewNoPrivacyWithdrawActionHandler(data code_data.Provider) ActionHandler {
	return &NoPrivacyWithdrawActionHandler{
		data: data,
	}
}

func (h *NoPrivacyWithdrawActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return errors.New("unexpected fulfillment type")
	}

	if newState == fulfillment.StateConfirmed {
		return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	if newState == fulfillment.StateFailed {
		return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	return nil
}

type PrivateTransferActionHandler struct {
	data code_data.Provider
}

func NewPrivateTransferActionHandler(data code_data.Provider) ActionHandler {
	return &PrivateTransferActionHandler{
		data: data,
	}
}

// There's many fulfillments for a private transfer action, so we define success
// and failure purely based on our ability to move funds to/from the user accounts.
func (h *PrivateTransferActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	switch fulfillmentRecord.FulfillmentType {
	case fulfillment.TemporaryPrivacyTransferWithAuthority, fulfillment.PermanentPrivacyTransferWithAuthority:
		if newState == fulfillment.StateConfirmed {
			return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		}

		if newState == fulfillment.StateFailed {
			return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		}
	case fulfillment.TransferWithCommitment:
		// No handling of confirmed state, since the other end of the split transfer
		// can't be run until the advance from the treasury is made to the destination.
		if newState == fulfillment.StateFailed {
			return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
		}
	case fulfillment.InitializeCommitmentProof, fulfillment.UploadCommitmentProof, fulfillment.VerifyCommitmentProof, fulfillment.OpenCommitmentVault, fulfillment.CloseCommitmentVault:
		// Don't care about commitment states. These are managed elsewhere.
		return nil
	default:
		return errors.New("unexpected fulfillment type")
	}

	return nil
}

type SaveRecentRootActionHandler struct {
	data code_data.Provider
}

func NewSaveRecentRootActionHandler(data code_data.Provider) ActionHandler {
	return &SaveRecentRootActionHandler{
		data: data,
	}
}

func (h *SaveRecentRootActionHandler) OnFulfillmentStateChange(ctx context.Context, fulfillmentRecord *fulfillment.Record, newState fulfillment.State) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.SaveRecentRoot {
		return errors.New("unexpected fulfillment type")
	}

	if newState == fulfillment.StateConfirmed {
		return markActionConfirmed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	if newState == fulfillment.StateFailed {
		return markActionFailed(ctx, h.data, fulfillmentRecord.Intent, fulfillmentRecord.ActionId)
	}

	return nil
}

func validateActionState(record *action.Record, states ...action.State) error {
	for _, validState := range states {
		if record.State == validState {
			return nil
		}
	}
	return ErrInvalidActionStateTransition
}

func markActionConfirmed(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	record, err := data.GetActionById(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if record.State == action.StateConfirmed {
		return nil
	}

	err = validateActionState(record, action.StatePending)
	if err != nil {
		return err
	}

	record.State = action.StateConfirmed
	return data.UpdateAction(ctx, record)
}

func markActionFailed(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	record, err := data.GetActionById(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if record.State == action.StateFailed {
		return nil
	}

	err = validateActionState(record, action.StatePending)
	if err != nil {
		return err
	}

	record.State = action.StateFailed
	return data.UpdateAction(ctx, record)
}

func markActionRevoked(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) error {
	record, err := data.GetActionById(ctx, intentId, actionId)
	if err != nil {
		return err
	}

	if record.State == action.StateRevoked {
		return nil
	}

	err = validateActionState(record, action.StateUnknown)
	if err != nil {
		return err
	}

	record.State = action.StateRevoked
	return data.UpdateAction(ctx, record)
}

func getActionHandlers(data code_data.Provider) map[action.Type]ActionHandler {
	handlersByType := make(map[action.Type]ActionHandler)
	handlersByType[action.OpenAccount] = NewOpenAccountActionHandler(data)
	handlersByType[action.CloseEmptyAccount] = NewCloseEmptyAccountActionHandler(data)
	handlersByType[action.NoPrivacyTransfer] = NewNoPrivacyTransferActionHandler(data)
	handlersByType[action.NoPrivacyWithdraw] = NewNoPrivacyWithdrawActionHandler(data)
	handlersByType[action.PrivateTransfer] = NewPrivateTransferActionHandler(data)
	handlersByType[action.SaveRecentRoot] = NewSaveRecentRootActionHandler(data)
	return handlersByType
}
