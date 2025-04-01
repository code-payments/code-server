package async_sequencer

import (
	"context"
	"errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
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

type SendPublicPaymentIntentHandler struct {
	data code_data.Provider
}

func NewSendPublicPaymentIntentHandler(data code_data.Provider) IntentHandler {
	return &SendPublicPaymentIntentHandler{
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
	handlersByType[intent.SendPublicPayment] = NewSendPublicPaymentIntentHandler(data)
	handlersByType[intent.ReceivePaymentsPublicly] = NewReceivePaymentsPubliclyIntentHandler(data)
	return handlersByType
}
