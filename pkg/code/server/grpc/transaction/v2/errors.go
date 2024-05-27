package transaction_v2

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/antispam"
	"github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	maxReasonStringLength = 2048
)

var (
	ErrTimedOutReceivingRequest = errors.New("timed out receiving request")

	ErrNotPhoneVerified         = newIntentDeniedError("not phone verified")
	ErrTooManyPayments          = newIntentDeniedError("too many payments")
	ErrTooManyNewRelationships  = newIntentDeniedError("too many new relationships")
	ErrTransactionLimitExceeded = newIntentDeniedError("dollar value exceeds limit")
	ErrNotManagedByCode         = newIntentDeniedError("at least one account is no longer managed by code")

	ErrInvalidSignature  = errors.New("invalid signature provided")
	ErrMissingSignature  = errors.New("at least one signature is missing")
	ErrTooManySignatures = errors.New("too many signatures provided")

	ErrInvalidActionToUpgrade = errors.New("attempt to upgrade an action that isn't upgradeable")
	ErrPrivacyUpgradeMissed   = errors.New("opportunity to upgrade the private transaction was missed")
	ErrPrivacyAlreadyUpgraded = errors.New("private transaction has already been upgraded")
	ErrWaitForNextBlock       = errors.New("must wait for next block before attempting privacy upgrade")

	ErrNotImplemented = errors.New("feature not implemented")
)

type IntentValidationError struct {
	message string
}

func newIntentValidationError(message string) IntentValidationError {
	return IntentValidationError{
		message: message,
	}
}

func newIntentValidationErrorf(format string, args ...any) IntentValidationError {
	return newIntentValidationError(fmt.Sprintf(format, args...))
}

func newActionValidationError(action *transactionpb.Action, message string) IntentValidationError {
	return newIntentValidationError(fmt.Sprintf("actions[%d]: %s", action.Id, message))
}

func newActionValidationErrorf(action *transactionpb.Action, message string, args ...any) IntentValidationError {
	return newActionValidationError(action, fmt.Sprintf(message, args...))
}

func (e IntentValidationError) Error() string {
	return e.message
}

type IntentDeniedError struct {
	message string
	reason  antispam.Reason
}

func newIntentDeniedError(message string) IntentDeniedError {
	return IntentDeniedError{
		message: message,
		reason:  antispam.ReasonUnspecified,
	}
}

func newIntentDeniedErrorWithAntispamReason(reason antispam.Reason, message string) IntentDeniedError {
	return IntentDeniedError{
		message: message,
		reason:  reason,
	}
}

func newIntentDeniedErrorf(reason antispam.Reason, format string, args ...any) IntentDeniedError {
	return newIntentDeniedError(fmt.Sprintf(format, args...))
}

func (e IntentDeniedError) Error() string {
	return e.message
}

type SwapValidationError struct {
	message string
}

func newSwapValidationError(message string) SwapValidationError {
	return SwapValidationError{
		message: message,
	}
}

func newSwapValidationErrorf(format string, args ...any) SwapValidationError {
	return newSwapValidationError(fmt.Sprintf(format, args...))
}

func (e SwapValidationError) Error() string {
	return e.message
}

type SwapDeniedError struct {
	message string
	reason  antispam.Reason
}

func newSwapDeniedError(message string) SwapDeniedError {
	return SwapDeniedError{
		message: message,
		reason:  antispam.ReasonUnspecified,
	}
}

func newSwapDeniedErrorf(format string, args ...any) SwapDeniedError {
	return newSwapDeniedError(fmt.Sprintf(format, args...))
}

func (e SwapDeniedError) Error() string {
	return e.message
}

type StaleStateError struct {
	message string
}

func newStaleStateError(message string) StaleStateError {
	return StaleStateError{
		message: message,
	}
}

func newStaleStateErrorf(format string, args ...any) StaleStateError {
	return newStaleStateError(fmt.Sprintf(format, args...))
}

func newActionWithStaleStateError(action *transactionpb.Action, message string) StaleStateError {
	return newStaleStateError(fmt.Sprintf("actions[%d]: %s", action.Id, message))
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

func toInvalidSignatureErrorDetails(
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
				ExpectedTransaction: &commonpb.Transaction{
					Value: txn.Marshal(),
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

	var antispamReason antispam.Reason
	switch typed := err.(type) {
	case IntentDeniedError:
		antispamReason = typed.reason
	case SwapDeniedError:
		antispamReason = typed.reason
	default:
		antispamReason = antispam.ReasonUnspecified
	}

	var code transactionpb.DeniedErrorDetails_Code
	switch antispamReason {
	case antispam.ReasonUnsupportedCountry:
		code = transactionpb.DeniedErrorDetails_UNSUPPORTED_COUNTRY
	case antispam.ReasonUnsupportedDevice:
		code = transactionpb.DeniedErrorDetails_UNSUPPORTED_DEVICE
	case antispam.ReasonTooManyFreeAccountsForPhoneNumber:
		code = transactionpb.DeniedErrorDetails_TOO_MANY_FREE_ACCOUNTS_FOR_PHONE_NUMBER
	case antispam.ReasonTooManyFreeAccountsForDevice:
		code = transactionpb.DeniedErrorDetails_TOO_MANY_FREE_ACCOUNTS_FOR_DEVIEC
	default:
		code = transactionpb.DeniedErrorDetails_UNSPECIFIED
	}

	return &transactionpb.ErrorDetails{
		Type: &transactionpb.ErrorDetails_Denied{
			Denied: &transactionpb.DeniedErrorDetails{
				Code:   code,
				Reason: reasonString,
			},
		},
	}
}

func handleSubmitIntentError(streamer transactionpb.Transaction_SubmitIntentServer, err error) error {
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
	case transaction.ErrNoAvailableNonces:
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

func handleSwapError(streamer transactionpb.Transaction_SwapServer, err error) error {
	// gRPC status errors are passed through as is
	if _, ok := status.FromError(err); ok {
		return err
	}

	// Case 1: Errors that map to a Code error response
	switch err.(type) {
	case SwapValidationError:
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_INVALID_SWAP,
			toReasonStringErrorDetails(err),
		)
	case SwapDeniedError:
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_DENIED,
			toDeniedErrorDetails(err),
		)
	}

	switch err {
	case ErrInvalidSignature:
		return handleSwapStructuredError(
			streamer,
			transactionpb.SwapResponse_Error_SIGNATURE_ERROR,
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
	}
	return status.Error(codes.Internal, "rpc server failure")
}

func handleSwapStructuredError(streamer transactionpb.Transaction_SwapServer, code transactionpb.SwapResponse_Error_Code, errorDetails ...*transactionpb.ErrorDetails) error {
	errResp := &transactionpb.SwapResponse{
		Response: &transactionpb.SwapResponse_Error_{
			Error: &transactionpb.SwapResponse_Error{
				Code:         code,
				ErrorDetails: errorDetails,
			},
		},
	}
	return streamer.Send(errResp)
}
