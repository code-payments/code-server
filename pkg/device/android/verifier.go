package android

import (
	"context"

	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "device.android.verifier"
)

type androidDeviceVerifier struct {
}

// NewAndroidDeviceVerifier returns a new device.Verifier for Android devices
func NewAndroidDeviceVerifier() (device.Verifier, error) {
	return &androidDeviceVerifier{}, nil
}

// IsValid implements device.Verifier.IsValid
//
// todo: implement me
func (v *androidDeviceVerifier) IsValid(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValid")
	defer tracer.End()

	isValid, err := func() (bool, error) {
		return false, nil
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return isValid, err
}

// HasCreatedFreeAccount implements device.Verifier.HasCreatedFreeAccount
//
// todo: implement me
func (v *androidDeviceVerifier) HasCreatedFreeAccount(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "HasCreatedFreeAccount")
	defer tracer.End()

	hasFreeCreatedAccount, err := func() (bool, error) {
		return false, nil
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return hasFreeCreatedAccount, err
}

// MarkCreatedFreeAccount implements device.Verifier.MarkCreatedFreeAccount
//
// todo: implement me
func (v *androidDeviceVerifier) MarkCreatedFreeAccount(ctx context.Context, token string) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "MarkCreatedFreeAccount")
	defer tracer.End()

	err := func() error {
		return nil
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return err
}
