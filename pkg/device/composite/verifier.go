package composite

import (
	"context"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/grpc/client"
)

type compositeDeviceVerifier struct {
	// todo: Something with persistence like Redis to account for multi-server and deploys
	// todo: This may not be needed if there's a concept of a production token that has a more aggressive expiry
	usedTokenCache cache.Cache

	verifiers map[client.DeviceType]device.Verifier
}

// NewCompositeDeviceVerifier returns a new device.Verifier that intelligently
// selects device.Verifier instance to verify a token using the client's user
// agent. It also provides a layer of protection against replay attacks by
// enforcing a single use per token value.
//
// If an unknown device type is detected, or no device.Verifier is registered,
// then the device is deemed invalid.
func NewCompositeDeviceVerifier(verifiers map[client.DeviceType]device.Verifier) device.Verifier {
	if verifiers == nil {
		verifiers = make(map[client.DeviceType]device.Verifier)
	}
	return &compositeDeviceVerifier{
		usedTokenCache: cache.NewCache(100_000),
		verifiers:      verifiers,
	}
}

// IsValid implements device.Verifier.IsValid
func (v *compositeDeviceVerifier) IsValid(ctx context.Context, token string) (bool, string, error) {
	// Enforce one-time use tokens, even when the third-party verifier doesn't
	// do a good job of doing so.
	if _, ok := v.usedTokenCache.Retrieve(token); ok {
		return false, "device token already consumed", nil
	}

	verifier, ok := v.getDeviceVerifier(ctx)
	if !ok {
		return false, "no device verifier for user agent", nil
	}

	isValid, reason, err := verifier.IsValid(ctx, token)
	if isValid {
		v.usedTokenCache.Insert(token, true, 1)
	}
	return isValid, reason, err
}

// HasCreatedFreeAccount implements device.Verifier.HasCreatedFreeAccount
func (v *compositeDeviceVerifier) HasCreatedFreeAccount(ctx context.Context, token string) (bool, error) {
	verifier, ok := v.getDeviceVerifier(ctx)
	if !ok {
		return false, nil
	}
	return verifier.HasCreatedFreeAccount(ctx, token)
}

// MarkCreatedFreeAccount implements device.Verifier.MarkCreatedFreeAccount
func (v *compositeDeviceVerifier) MarkCreatedFreeAccount(ctx context.Context, token string) error {
	verifier, ok := v.getDeviceVerifier(ctx)
	if !ok {
		return nil
	}
	return verifier.MarkCreatedFreeAccount(ctx, token)
}

func (v *compositeDeviceVerifier) getDeviceVerifier(ctx context.Context) (device.Verifier, bool) {
	userAgent, err := client.GetUserAgent(ctx)
	if err != nil {
		return nil, false
	}

	if userAgent.DeviceType == client.DeviceTypeUnknown {
		return nil, false
	}

	verifier, ok := v.verifiers[userAgent.DeviceType]
	return verifier, ok
}
