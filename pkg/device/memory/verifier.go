package memory

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/device"
)

const (
	ValidDeviceToken   = "valid-device-token"
	InvalidDeviceToken = "invalid-device-token"
)

type memoryDeviceVerifier struct {
	createdFreeAccount bool
}

// NewMemoryDeviceVerifier returns a new device.Verifier for testing
func NewMemoryDeviceVerifier() device.Verifier {
	return &memoryDeviceVerifier{}
}

// IsValid implements device.Verifier.IsValid
func (v *memoryDeviceVerifier) IsValid(_ context.Context, token string) (bool, string, error) {
	var reason string
	if token != ValidDeviceToken {
		reason = "invalid test device token"
	}

	return token == ValidDeviceToken, reason, nil
}

// HasCreatedFreeAccount implements device.Verifier.HasCreatedFreeAccount
func (v *memoryDeviceVerifier) HasCreatedFreeAccount(ctx context.Context, token string) (bool, error) {
	if token != ValidDeviceToken {
		return false, errors.New("invalid device token")
	}

	return v.createdFreeAccount, nil
}

// MarkCreatedFreeAccount implements device.Verifier.MarkCreatedFreeAccount
func (v *memoryDeviceVerifier) MarkCreatedFreeAccount(ctx context.Context, token string) error {
	if token != ValidDeviceToken {
		return errors.New("invalid device token")
	}

	v.createdFreeAccount = true

	return nil
}
