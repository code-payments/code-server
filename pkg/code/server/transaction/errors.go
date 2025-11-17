package transaction_v2

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

const (
	maxReasonStringLength = 2048
)

var (
	ErrTimedOutReceivingRequest = errors.New("timed out receiving request")

	ErrTooManyPayments             = NewIntentDeniedError("too many payments")
	ErrTransactionLimitExceeded    = NewIntentDeniedError("dollar value exceeds limit")
	ErrSourceNotManagedByCode      = NewIntentDeniedError("at least one source account is no longer managed by code")
	ErrDestinationNotManagedByCode = NewIntentDeniedError("a destination account is no longer managed by code")

	ErrInvalidSignature  = errors.New("invalid signature provided")
	ErrMissingSignature  = errors.New("at least one signature is missing")
	ErrTooManySignatures = errors.New("too many signatures provided")

	ErrNotImplemented = errors.New("feature not implemented")
)

type IntentValidationError struct {
	message string
}

func NewIntentValidationError(message string) IntentValidationError {
	return IntentValidationError{
		message: message,
	}
}

func NewIntentValidationErrorf(format string, args ...any) IntentValidationError {
	return NewIntentValidationError(fmt.Sprintf(format, args...))
}

func NewActionValidationError(action *transactionpb.Action, message string) IntentValidationError {
	return NewIntentValidationError(fmt.Sprintf("actions[%d]: %s", action.Id, message))
}

func NewActionValidationErrorf(action *transactionpb.Action, message string, args ...any) IntentValidationError {
	return NewActionValidationError(action, fmt.Sprintf(message, args...))
}

func (e IntentValidationError) Error() string {
	return e.message
}

type IntentDeniedError struct {
	message string
}

func NewIntentDeniedError(message string) IntentDeniedError {
	return IntentDeniedError{
		message: message,
	}
}

func (e IntentDeniedError) Error() string {
	return e.message
}

type StaleStateError struct {
	message string
}

func NewStaleStateError(message string) StaleStateError {
	return StaleStateError{
		message: message,
	}
}

func NewStaleStateErrorf(format string, args ...any) StaleStateError {
	return NewStaleStateError(fmt.Sprintf(format, args...))
}

func NewActionWithStaleStateError(action *transactionpb.Action, message string) StaleStateError {
	return NewStaleStateError(fmt.Sprintf("actions[%d]: %s", action.Id, message))
}

func (e StaleStateError) Error() string {
	return e.message
}

func toReasonStringErrorDetails(err error) *transactionpb.ErrorDetails {
	if err == nil {
		return nil
	}

	reasonString := err.Error()
	if len(reasonString) > maxReasonStringLength {
		reasonString = reasonString[:maxReasonStringLength]
	}

	return &transactionpb.ErrorDetails{
		Type: &transactionpb.ErrorDetails_ReasonString{
			ReasonString: &transactionpb.ReasonStringErrorDetails{
				Reason: reasonString,
			},
		},
	}
}

func toInvalidTxnSignatureErrorDetails(
	actionId uint32,
	txn solana.Transaction,
	signature *commonpb.Signature,
) *transactionpb.ErrorDetails {
	// Clear out all signatures, so clients have no way of submitting this transaction
	var emptySig solana.Signature
	for i := range txn.Signatures {
		copy(txn.Signatures[i][:], emptySig[:])
	}

	return &transactionpb.ErrorDetails{
		Type: &transactionpb.ErrorDetails_InvalidSignature{
			InvalidSignature: &transactionpb.InvalidSignatureErrorDetails{
				ActionId: actionId,
				ExpectedBlob: &transactionpb.InvalidSignatureErrorDetails_ExpectedTransaction{
					ExpectedTransaction: &commonpb.Transaction{
						Value: txn.Marshal(),
					},
				},
				ProvidedSignature: signature,
			},
		},
	}
}

func toInvalidVirtualIxnSignatureErrorDetails(
	actionId uint32,
	virtualIxnHash cvm.CompactMessage,
	signature *commonpb.Signature,
) *transactionpb.ErrorDetails {
	return &transactionpb.ErrorDetails{
		Type: &transactionpb.ErrorDetails_InvalidSignature{
			InvalidSignature: &transactionpb.InvalidSignatureErrorDetails{
				ActionId: actionId,
				ExpectedBlob: &transactionpb.InvalidSignatureErrorDetails_ExpectedVixnHash{
					ExpectedVixnHash: &commonpb.Hash{
						Value: virtualIxnHash[:],
					},
				},
				ProvidedSignature: signature,
			},
		},
	}
}

func toDeniedErrorDetails(err error) *transactionpb.ErrorDetails {
	if err == nil {
		return nil
	}

	reasonString := err.Error()
	if len(reasonString) > maxReasonStringLength {
		reasonString = reasonString[:maxReasonStringLength]
	}

	return &transactionpb.ErrorDetails{
		Type: &transactionpb.ErrorDetails_Denied{
			Denied: &transactionpb.DeniedErrorDetails{
				Code:   transactionpb.DeniedErrorDetails_UNSPECIFIED,
				Reason: reasonString,
			},
		},
	}
}

func handleSubmitIntentError(ctx context.Context, streamer transactionpb.Transaction_SubmitIntentServer, intentRecord *intent.Record, err error) error {
	if !shouldFilterSubmitIntentFailureMetricReport(err) {
		recordCriticalSubmitIntentFailure(ctx, intentRecord, err)
	}

	// gRPC status errors are passed through as is
	if _, ok := status.FromError(err); ok {
		return err
	}

	// Case 1: Errors that map to a Code error response
	switch err.(type) {
	case IntentValidationError:
		return handleSubmitIntentStructuredError(
			streamer,
			transactionpb.SubmitIntentResponse_Error_INVALID_INTENT,
			toReasonStringErrorDetails(err),
		)
	case IntentDeniedError:
		return handleSubmitIntentStructuredError(
			streamer,
			transactionpb.SubmitIntentResponse_Error_DENIED,
			toDeniedErrorDetails(err),
		)
	case StaleStateError:
		return handleSubmitIntentStructuredError(
			streamer,
			transactionpb.SubmitIntentResponse_Error_STALE_STATE,
			toReasonStringErrorDetails(err),
		)
	}

	switch err {
	case ErrMissingSignature, ErrTooManySignatures, ErrInvalidSignature:
		return handleSubmitIntentStructuredError(
			streamer,
			transactionpb.SubmitIntentResponse_Error_SIGNATURE_ERROR,
			toReasonStringErrorDetails(err),
		)
	case ErrNotImplemented:
		return status.Error(codes.Unimplemented, err.Error())
	}

	// Case 2: Errors that map to gRPC status errors
	switch err {
	case ErrTimedOutReceivingRequest, context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, err.Error())
	case context.Canceled:
		return status.Error(codes.Canceled, err.Error())
	case transaction.ErrNoAvailableNonces, transaction.ErrNoncePoolNotFound:
		return status.Error(codes.Unavailable, "")
	}
	return status.Error(codes.Internal, "rpc server failure")
}

func handleSubmitIntentStructuredError(streamer transactionpb.Transaction_SubmitIntentServer, code transactionpb.SubmitIntentResponse_Error_Code, errorDetails ...*transactionpb.ErrorDetails) error {
	errResp := &transactionpb.SubmitIntentResponse{
		Response: &transactionpb.SubmitIntentResponse_Error_{
			Error: &transactionpb.SubmitIntentResponse_Error{
				Code:         code,
				ErrorDetails: errorDetails,
			},
		},
	}
	return streamer.Send(errResp)
}

func shouldFilterSubmitIntentFailureMetricReport(err error) bool {
	if statusErr, ok := status.FromError(err); ok {
		switch statusErr.Code() {
		case codes.Canceled:
			return true
		}
	}

	// todo: something more scalable
	for _, filteredSubstr := range []string{
		context.Canceled.Error(),
		"gift card balance has already been claimed",
	} {
		if strings.Contains(strings.ToLower(err.Error()), filteredSubstr) {
			return true
		}
	}

	return false
}
