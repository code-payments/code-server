package device

import "context"

type Verifier interface {
	// IsValid determines a device's validity using an opaque device token.
	IsValid(ctx context.Context, token string) (bool, string, error)

	// HasCreatedFreeAccount verifies whether the device has created a free account
	HasCreatedFreeAccount(ctx context.Context, token string) (bool, error)

	// MarkCreatedFreeAccount marks the device as having created a free account
	MarkCreatedFreeAccount(ctx context.Context, token string) error
}
