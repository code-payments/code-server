package phone

import (
	"context"
	"errors"
)

var (
	// ErrInvalidNumber is returned if the phone number is invalid
	ErrInvalidNumber = errors.New("phone number is invalid")

	// ErrRateLimited indicates that the call was rate limited
	ErrRateLimited = errors.New("rate limited")

	// ErrInvalidVerificationCode is returned when the verification does not
	// match what's expected in a Confirm call
	ErrInvalidVerificationCode = errors.New("verification code does not match")

	// ErrNoVerification indicates that no verification is in progress for
	// the provided phone number in a Confirm call. Several reasons this
	// can occur include a verification being expired or having reached a
	// maximum check threshold.
	ErrNoVerification = errors.New("verification not in progress")

	// ErrUnsupportedPhoneType indicates the provided phone number maps to
	// a type of phone that isn't supported.
	ErrUnsupportedPhoneType = errors.New("unsupported phone type")
)

type Verifier interface {
	// SendCode sends a verification code via SMS to the provided phone number.
	// If an active verification is already taking place, the existing code will
	// be resent. A unique ID for the verification and phone metadata is returned on
	// success.
	SendCode(ctx context.Context, phoneNumber string) (string, *Metadata, error)

	// Check verifies a SMS code sent to a phone number.
	Check(ctx context.Context, phoneNumber, code string) error

	// Cancel cancels an active verification. No error is returned if the
	// verification doesn't exist, previously canceled or successfully
	// completed.
	Cancel(ctx context.Context, id string) error

	// IsVerificationActive checks whether a verification is active or not
	IsVerificationActive(ctx context.Context, id string) (bool, error)

	// IsValid validates a phone number is real and is able to receive
	// a verification code.
	IsValidPhoneNumber(ctx context.Context, phoneNumber string) (bool, error)
}
