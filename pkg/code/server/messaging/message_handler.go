package messaging

import (
	"context"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
)

// MessageHandler provides message-specific in addition to the generic message
// handling flows. Implementations are responsible for determining whether a
// message can be allowed to be retried.
type MessageHandler interface {
	// Validate validates a message, which determines whether it should be
	// allowed to be sent and persisted
	Validate(ctx context.Context, rendezvous *common.Account, message *messagingpb.Message) error

	// OnSuccess is called upon creating the message after validation
	OnSuccess(ctx context.Context) error
}

type RequestToGrabBillMessageHandler struct {
	data code_data.Provider
}

func NewRequestToGrabBillMessageHandler(data code_data.Provider) MessageHandler {
	return &RequestToGrabBillMessageHandler{
		data: data,
	}
}

func (h *RequestToGrabBillMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetRequestToGrabBill()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	//
	// Part 1: Requestor account must be a primary account
	//

	requestorAccount, err := common.NewAccountFromProto(typedMessage.RequestorAccount)
	if err != nil {
		return err
	}

	accountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, requestorAccount.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound || (err == nil && accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY) {
		return newMessageValidationError("requestor account must be a primary account")
	} else if err != nil {
		return err
	}

	return nil
}

func (h *RequestToGrabBillMessageHandler) OnSuccess(ctx context.Context) error {
	return nil
}
