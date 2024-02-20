package async_sequencer

import (
	"context"
	"errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

var (
	ErrInvalidIntentStateTransition = errors.New("invalid intent state transition")
)

type IntentHandler interface {
	// Note: As of right now, we're restricted to checking actions, whose state is
	//       guaranteed to be updated by the time this is called. As a result, the
	//       intent handler should just get the records as needed since they need
	//       a global view anyways.
	OnActionUpdated(ctx context.Context, intentId string) error
}

type OpenAccountsIntentHandler struct {
	data code_data.Provider
}

func NewOpenAccountsIntentHandler(data code_data.Provider) IntentHandler {
	return &OpenAccountsIntentHandler{
		data: data,
	}
}

func (h *OpenAccountsIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecords, err := h.data.GetAllActionsByIntent(ctx, intentId)
	if err != nil {
		return err
	}

	for _, actionRecord := range actionRecords {
		if actionRecord.ActionType != action.OpenAccount {
			continue
		}

		// Intent is failed if at least one OpenAccount action fails
		if actionRecord.State == action.StateFailed {
			return markIntentFailed(ctx, h.data, intentId)
		}

		if actionRecord.State != action.StateConfirmed {
			return nil
		}
	}

	// Intent is confirmed when all OpenAccount actions are confirmed
	return markIntentConfirmed(ctx, h.data, intentId)
}

type SendPrivatePaymentIntentHandler struct {
	data code_data.Provider
}

func NewSendPrivatePaymentIntentHandler(data code_data.Provider) IntentHandler {
	return &SendPrivatePaymentIntentHandler{
		data: data,
	}
}

func (h *SendPrivatePaymentIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecords, err := h.data.GetAllActionsByIntent(ctx, intentId)
	if err != nil {
		return err
	}

	canMarkConfirmed := true
	for _, actionRecord := range actionRecords {
		switch actionRecord.ActionType {
		case action.PrivateTransfer, action.NoPrivacyTransfer, action.NoPrivacyWithdraw:
		default:
			continue
		}

		// Intent is failed if at least one money movement action fails
		if actionRecord.State == action.StateFailed {
			return markIntentFailed(ctx, h.data, intentId)
		}

		if actionRecord.State != action.StateConfirmed {
			canMarkConfirmed = false
		}
	}

	// Intent is confirmed when all money movement actions are confirmed
	if canMarkConfirmed {
		return markIntentConfirmed(ctx, h.data, intentId)
	}
	return h.maybeMarkTempOutgoingAccountActionsAsActivelyScheduled(ctx, intentId, actionRecords)
}

func (h *SendPrivatePaymentIntentHandler) maybeMarkTempOutgoingAccountActionsAsActivelyScheduled(ctx context.Context, intentId string, actionsRecords []*action.Record) error {
	intentRecord, err := h.data.GetIntent(ctx, intentId)
	if err != nil {
		return err
	}

	// Find relevant actions that have fulfillments using the temp outgoing account
	// where active scheduling is disabled because of treasury advance dependencies.
	//
	// Note: SubmitIntent validation guarantees there's a single NoPrivacyWithdraw
	// and NoPrivacyTransfer action that maps to the payment to the destianattion
	// account and optional fee to Code, respectively, all coming from the temp
	// outgoing account.
	var paymentToDestinationAction *action.Record
	var feePaymentActions []*action.Record
	for _, actionRecord := range actionsRecords {
		switch actionRecord.ActionType {
		case action.NoPrivacyWithdraw:
			paymentToDestinationAction = actionRecord
		case action.NoPrivacyTransfer:
			feePaymentActions = append(feePaymentActions, actionRecord)
		}
	}

	if paymentToDestinationAction == nil {
		return errors.New("payment to destination action not found")
	}
	if len(feePaymentActions) == 0 && intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
		return errors.New("fee payment action not found")
	}

	// Extract the corresponding fulfillment records that have active scheduling
	// disabled.

	var paymentToDestinationFulfillment *fulfillment.Record
	fulfillmentRecords, err := h.data.GetAllFulfillmentsByTypeAndAction(
		ctx,
		fulfillment.NoPrivacyWithdraw,
		intentId,
		paymentToDestinationAction.ActionId,
	)
	if err != nil {
		return err
	} else if err == nil && fulfillmentRecords[0].DisableActiveScheduling {
		paymentToDestinationFulfillment = fulfillmentRecords[0]
	}

	var feePaymentFulfillments []*fulfillment.Record
	for _, feePaymentAction := range feePaymentActions {
		fulfillmentRecords, err := h.data.GetAllFulfillmentsByTypeAndAction(
			ctx,
			fulfillment.NoPrivacyTransferWithAuthority,
			intentId,
			feePaymentAction.ActionId,
		)
		if err != nil {
			return err
		} else if err == nil && fulfillmentRecords[0].DisableActiveScheduling {
			feePaymentFulfillments = append(feePaymentFulfillments, fulfillmentRecords[0])
		}
	}

	// Short circuit if there's nothing to update to avoid redundant intent
	// state checks spanning all actions.
	if paymentToDestinationFulfillment == nil && len(feePaymentFulfillments) == 0 {
		return nil
	}

	// Do some sanity checks to determine whether active scheduling can be enabled.
	tempOutgoingAccount := paymentToDestinationAction.Source
	for _, actionRecord := range actionsRecords {
		if actionRecord.ActionType != action.PrivateTransfer {
			continue
		}

		if *actionRecord.Destination != tempOutgoingAccount {
			continue
		}

		// Is there a treasury advance that's sending funds to the temp outgoing
		// account that's not pending or completed? If so, wait for all advances
		// to be scheduled or completed. We need to rely on fulfillments because
		// private transfer action states operate on the entire lifecycle, and we
		// only care about the treasury advances.
		transferWithCommitmentFulfillment, err := h.data.GetAllFulfillmentsByTypeAndAction(
			ctx,
			fulfillment.TransferWithCommitment,
			intentId,
			actionRecord.ActionId,
		)
		if err != nil {
			return err
		}

		// Note: Due to how the generic fulfillment worker logic functions, it's
		// likely that at least one fulfillment is in an in flux state from pending
		// to confirmed. This is by design to allow the worker to retry, but makes
		// this logic imperfect by just checking for all confirmed. That's why it
		// differs from other intent handlers that can operate on actions, since
		// that flow has these guarantees. Regardless, dependency logic still saves
		// us and we're only making the fulfillment available for active polling.
		if transferWithCommitmentFulfillment[0].State != fulfillment.StatePending && transferWithCommitmentFulfillment[0].State != fulfillment.StateConfirmed {
			return nil
		}
	}

	// Mark fulfillments as actively scheduled when at least all treasury payments
	// to the temp outgoing account are in flight.
	for _, feePaymentFulfillment := range feePaymentFulfillments {
		err = markFulfillmentAsActivelyScheduled(ctx, h.data, feePaymentFulfillment)
		if err != nil {
			return err
		}
	}
	if paymentToDestinationFulfillment != nil {
		err = markFulfillmentAsActivelyScheduled(ctx, h.data, paymentToDestinationFulfillment)
		if err != nil {
			return err
		}
	}
	return nil
}

type ReceivePaymentsPrivatelyIntentHandler struct {
	data code_data.Provider
}

func NewReceivePaymentsPrivatelyIntentHandler(data code_data.Provider) IntentHandler {
	return &ReceivePaymentsPrivatelyIntentHandler{
		data: data,
	}
}

func (h *ReceivePaymentsPrivatelyIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecords, err := h.data.GetAllActionsByIntent(ctx, intentId)
	if err != nil {
		return err
	}

	canMarkConfirmed := true
	for _, actionRecord := range actionRecords {
		switch actionRecord.ActionType {
		case action.PrivateTransfer, action.CloseEmptyAccount:
		default:
			continue
		}

		// Intent is failed if at least one PrivateTransfer action fails
		if actionRecord.State == action.StateFailed {
			return markIntentFailed(ctx, h.data, intentId)
		}

		if actionRecord.State != action.StateConfirmed {
			canMarkConfirmed = false
		}
	}

	// Intent is confirmed when all PrivateTransfer and CloseEmptyAccount (there should only
	// be one when receiving from temp incoming) actions are confirmed
	if canMarkConfirmed {
		return markIntentConfirmed(ctx, h.data, intentId)
	}
	return h.maybeMarkCloseEmptyAccountActionAsActivelyScheduled(ctx, intentId, actionRecords)
}

func (h *ReceivePaymentsPrivatelyIntentHandler) maybeMarkCloseEmptyAccountActionAsActivelyScheduled(ctx context.Context, intentId string, actionsRecords []*action.Record) error {
	intentRecord, err := h.data.GetIntent(ctx, intentId)
	if err != nil {
		return err
	}

	// Deposits don't have a CloseEmptyAccount action because they receive from a
	// persistent primary account
	if intentRecord.ReceivePaymentsPrivatelyMetadata.IsDeposit {
		return nil
	}

	var closeEmptyAccountAction *action.Record
	for _, actionRecord := range actionsRecords {
		if actionRecord.ActionType == action.CloseEmptyAccount {
			closeEmptyAccountAction = actionRecord
			break
		}
	}

	if closeEmptyAccountAction == nil {
		return errors.New("close empty account action not found")
	}

	tempIncomingAccount := closeEmptyAccountAction.Source

	// Is there an unconfirmed private transfer that's dependent on the account
	// being closed as a source of funds? If so, wait for it to complete to drain
	// the balance.
	for _, actionRecord := range actionsRecords {
		if actionRecord.ActionType != action.PrivateTransfer {
			continue
		}

		if actionRecord.Source != tempIncomingAccount {
			continue
		}

		if actionRecord.State != action.StateConfirmed {
			return nil
		}
	}

	// All private transfers from the temp incoming account are confirmed. There
	// should be no funds (except possibly some dust), so w can actively schedule
	// to close fulfillment.

	// There should only be one per intent validation in SubmitIntent
	closeEmptyAccountFulfillment, err := h.data.GetAllFulfillmentsByTypeAndAction(
		ctx,
		fulfillment.CloseEmptyTimelockAccount,
		closeEmptyAccountAction.Intent,
		closeEmptyAccountAction.ActionId,
	)
	if err != nil {
		return err
	}
	return markFulfillmentAsActivelyScheduled(ctx, h.data, closeEmptyAccountFulfillment[0])
}

type SaveRecentRootIntentHandler struct {
	data code_data.Provider
}

func NewSaveRecentRootIntentHandler(data code_data.Provider) IntentHandler {
	return &SaveRecentRootIntentHandler{
		data: data,
	}
}

func (h *SaveRecentRootIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecord, err := h.data.GetActionById(ctx, intentId, 0)
	if err != nil {
		return err
	}

	// Intent is confirmed/failed based on the state the single action
	switch actionRecord.State {
	case action.StateConfirmed:
		return markIntentConfirmed(ctx, h.data, intentId)
	case action.StateFailed:
		return markIntentFailed(ctx, h.data, intentId)
	}
	return nil
}

type MigrateToPrivacy2022IntentHandler struct {
	data code_data.Provider
}

func NewMigrateToPrivacy2022IntentHandler(data code_data.Provider) IntentHandler {
	return &MigrateToPrivacy2022IntentHandler{
		data: data,
	}
}

func (h *MigrateToPrivacy2022IntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecord, err := h.data.GetActionById(ctx, intentId, 0)
	if err != nil {
		return err
	}

	// Intent is confirmed/failed based on the state the single action
	switch actionRecord.State {
	case action.StateConfirmed:
		return markIntentConfirmed(ctx, h.data, intentId)
	case action.StateFailed:
		return markIntentFailed(ctx, h.data, intentId)
	}
	return nil
}

type SendPublicPaymentIntentHandler struct {
	data code_data.Provider
}

func NewSendPublicPaymentIntentHandler(data code_data.Provider) IntentHandler {
	return &MigrateToPrivacy2022IntentHandler{
		data: data,
	}
}

func (h *SendPublicPaymentIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecord, err := h.data.GetActionById(ctx, intentId, 0)
	if err != nil {
		return err
	}

	// Intent is confirmed/failed based on the state the single action
	switch actionRecord.State {
	case action.StateConfirmed:
		return markIntentConfirmed(ctx, h.data, intentId)
	case action.StateFailed:
		return markIntentFailed(ctx, h.data, intentId)
	}
	return nil
}

type ReceivePaymentsPubliclyIntentHandler struct {
	data code_data.Provider
}

func NewReceivePaymentsPubliclyIntentHandler(data code_data.Provider) IntentHandler {
	return &ReceivePaymentsPubliclyIntentHandler{
		data: data,
	}
}

func (h *ReceivePaymentsPubliclyIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecord, err := h.data.GetActionById(ctx, intentId, 0)
	if err != nil {
		return err
	}

	// Intent is confirmed/failed based on the state the single action
	switch actionRecord.State {
	case action.StateConfirmed:
		return markIntentConfirmed(ctx, h.data, intentId)
	case action.StateFailed:
		return markIntentFailed(ctx, h.data, intentId)
	}
	return nil
}

type EstablishRelationshipIntentHandler struct {
	data code_data.Provider
}

func NewEstablishRelationshipIntentHandler(data code_data.Provider) IntentHandler {
	return &EstablishRelationshipIntentHandler{
		data: data,
	}
}

func (h *EstablishRelationshipIntentHandler) OnActionUpdated(ctx context.Context, intentId string) error {
	actionRecord, err := h.data.GetActionById(ctx, intentId, 0)
	if err != nil {
		return err
	}

	// Intent is confirmed/failed based on the state the single action
	switch actionRecord.State {
	case action.StateConfirmed:
		return markIntentConfirmed(ctx, h.data, intentId)
	case action.StateFailed:
		return markIntentFailed(ctx, h.data, intentId)
	}
	return nil
}

func validateIntentState(record *intent.Record, states ...intent.State) error {
	for _, validState := range states {
		if record.State == validState {
			return nil
		}
	}
	return ErrInvalidIntentStateTransition
}

func markIntentConfirmed(ctx context.Context, data code_data.Provider, intentId string) error {
	record, err := data.GetIntent(ctx, intentId)
	if err != nil {
		return err
	}

	if record.State == intent.StateConfirmed {
		return nil
	}

	err = validateIntentState(record, intent.StatePending)
	if err != nil {
		return err
	}

	record.State = intent.StateConfirmed
	return data.SaveIntent(ctx, record)
}

func markIntentFailed(ctx context.Context, data code_data.Provider, intentId string) error {
	record, err := data.GetIntent(ctx, intentId)
	if err != nil {
		return err
	}

	if record.State == intent.StateFailed {
		return nil
	}

	err = validateIntentState(record, intent.StatePending)
	if err != nil {
		return err
	}

	record.State = intent.StateFailed
	return data.SaveIntent(ctx, record)
}
func getIntentHandlers(data code_data.Provider) map[intent.Type]IntentHandler {
	handlersByType := make(map[intent.Type]IntentHandler)
	handlersByType[intent.OpenAccounts] = NewOpenAccountsIntentHandler(data)
	handlersByType[intent.SendPrivatePayment] = NewSendPrivatePaymentIntentHandler(data)
	handlersByType[intent.ReceivePaymentsPrivately] = NewReceivePaymentsPrivatelyIntentHandler(data)
	handlersByType[intent.SaveRecentRoot] = NewSaveRecentRootIntentHandler(data)
	handlersByType[intent.MigrateToPrivacy2022] = NewMigrateToPrivacy2022IntentHandler(data)
	handlersByType[intent.SendPublicPayment] = NewSendPublicPaymentIntentHandler(data)
	handlersByType[intent.ReceivePaymentsPublicly] = NewReceivePaymentsPubliclyIntentHandler(data)
	handlersByType[intent.EstablishRelationship] = NewEstablishRelationshipIntentHandler(data)
	return handlersByType
}
