package ios

import (
	"context"
	"errors"
	"strings"

	devicecheck "github.com/rinchsan/device-check-go"

	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "device.ios.verifier"
)

type AppleEnv uint8

const (
	AppleEnvDevelopment AppleEnv = iota
	AppleEnvProduction
)

// Current two-bit configuration:
//   - bit0: Free account flag
//   - bit1: Unused
//
// todo: May need a small refactor if bit1 is used
type iOSDeviceVerifier struct {
	client     *devicecheck.Client
	minVersion *client.Version
}

// NewIOSDeviceVerifier returns a new device.Verifier for iOS devices
func NewIOSDeviceVerifier(
	env AppleEnv,
	keyIssuer string,
	keyId string,
	privateKeyFile string,
	minVersion *client.Version,
) (device.Verifier, error) {
	var dcEnv devicecheck.Environment
	switch env {
	case AppleEnvDevelopment:
		dcEnv = devicecheck.Development
	case AppleEnvProduction:
		dcEnv = devicecheck.Production
	default:
		return nil, errors.New("invalid environment")
	}

	client := devicecheck.New(
		devicecheck.NewCredentialFile(privateKeyFile),
		devicecheck.NewConfig(keyIssuer, keyId, dcEnv),
	)
	return &iOSDeviceVerifier{
		client:     client,
		minVersion: minVersion,
	}, nil
}

// IsValid implements device.Verifier.IsValid
func (v *iOSDeviceVerifier) IsValid(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValid")
	defer tracer.End()

	isValid, err := func() (bool, error) {
		userAgent, err := client.GetUserAgent(ctx)
		if err != nil {
			return false, nil
		}

		if userAgent.DeviceType != client.DeviceTypeIOS {
			return false, nil
		}

		if userAgent.Version.Before(v.minVersion) {
			return false, nil
		}

		err = v.client.ValidateDeviceToken(token)
		if err == nil {
			return true, nil
		}

		// Need to parse for the "bad device token" type of errors. Otherwise, we
		// cannot distinguish between a validity issue or other API/network error.
		//
		// https://developer.apple.com/documentation/devicecheck/accessing_and_modifying_per-device_data#2910408
		errorString := strings.ToLower(err.Error())
		if strings.Contains(errorString, "bad device token") {
			return false, nil
		} else if strings.Contains(errorString, "missing or incorrectly formatted device token payload") {
			return false, nil
		}
		return false, err
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return isValid, err
}

// HasCreatedFreeAccount implements device.Verifier.HasCreatedFreeAccount
func (v *iOSDeviceVerifier) HasCreatedFreeAccount(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "HasCreatedFreeAccount")
	defer tracer.End()

	hasCreatedFreeAccount, err := func() (bool, error) {
		var res devicecheck.QueryTwoBitsResult
		err := v.client.QueryTwoBits(token, &res)
		if err == nil {
			return res.Bit0, nil
		}

		errorString := strings.ToLower(err.Error())
		if strings.Contains(errorString, "bit state not found") {
			return false, nil
		}
		return false, err
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return hasCreatedFreeAccount, err
}

// MarkCreatedFreeAccount implements device.Verifier.MarkCreatedFreeAccount
func (v *iOSDeviceVerifier) MarkCreatedFreeAccount(ctx context.Context, token string) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "MarkCreatedFreeAccount")
	defer tracer.End()

	err := v.client.UpdateTwoBits(token, true, false)
	if err != nil {
		tracer.OnError(err)
	}
	return err
}
