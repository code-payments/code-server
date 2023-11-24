package mock

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/code-payments/code-server/pkg/phone"
)

const (
	// ValidPhoneVerificationToken can be used to test successful confirmation
	// of active phone verifications.
	ValidPhoneVerificationToken = "123456"

	// InvalidPhoneVerificationToken can be used to test unsuccessful confirmation
	// of phone verifications.
	InvalidPhoneVerificationToken = "999999"
)

type verifier struct {
	mu sync.Mutex

	activeVerificationsByID     map[string]string
	activeVerificationsByNumber map[string]string
}

// NewVerifier returns a new in memory phone verifier that always "sends" a
// code with value 123456. Verifications are long lived and will be completed
// when either canceled or pass validation.
func NewVerifier() phone.Verifier {
	return &verifier{
		activeVerificationsByID:     make(map[string]string),
		activeVerificationsByNumber: make(map[string]string),
	}
}

// SendCode implements phone.Verifier.SendCode
func (v *verifier) SendCode(ctx context.Context, phoneNumber string) (string, *phone.Metadata, error) {
	if !phone.IsE164Format(phoneNumber) {
		return "", nil, phone.ErrInvalidNumber
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	metadata := &phone.Metadata{
		PhoneNumber: phoneNumber,
	}
	metadata.SetType(phone.TypeMobile)
	metadata.SetMobileCountryCode(302) // Canada
	metadata.SetMobileNetworkCode(720) // Rogers

	// There's already an active verification, so simulate re-sending the code
	if id, ok := v.activeVerificationsByNumber[phoneNumber]; ok {
		return id, metadata, nil
	}

	// Otherwise, create a new verification and simulate sending the code for the
	// first time
	id := uuid.New().String()
	v.activeVerificationsByID[id] = phoneNumber
	v.activeVerificationsByNumber[phoneNumber] = id

	return id, metadata, nil
}

// Check implements phone.Verifier.Check
func (v *verifier) Check(ctx context.Context, phoneNumber, code string) error {
	if !phone.IsVerificationCode(code) {
		return phone.ErrInvalidVerificationCode
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	id, ok := v.activeVerificationsByNumber[phoneNumber]

	// There's no active verifications
	if !ok {
		return phone.ErrNoVerification
	}

	// There's an active verification, but the code doesn't match
	if code != ValidPhoneVerificationToken {
		return phone.ErrInvalidVerificationCode
	}

	// The code matches and the verification is complete
	delete(v.activeVerificationsByID, id)
	delete(v.activeVerificationsByNumber, phoneNumber)

	return nil
}

// Cancel implements phone.Verifier.Cancel
func (v *verifier) Cancel(ctx context.Context, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	phoneNumber, ok := v.activeVerificationsByID[id]

	// There's no active verification for the phone number
	if !ok {
		return nil
	}

	// Simulate canceling the verification by removing the verification.
	delete(v.activeVerificationsByID, id)
	delete(v.activeVerificationsByNumber, phoneNumber)

	return nil
}

// IsVerificationActive implements phone.Verifier.IsVerificationActive
func (v *verifier) IsVerificationActive(ctx context.Context, id string) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	_, ok := v.activeVerificationsByID[id]
	return ok, nil
}

// IsValidPhoneNumber implements phone.Verifier.IsValidPhoneNumber
func (v *verifier) IsValidPhoneNumber(_ context.Context, phoneNumber string) (bool, error) {
	return phone.IsE164Format(phoneNumber), nil
}
