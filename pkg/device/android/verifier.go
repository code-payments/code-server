package android

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/playintegrity/v1"

	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "device.android.verifier"
)

type androidDeviceVerifier struct {
	playIntegrity *playintegrity.Service
	packageName   string
	minVersion    int64
}

// NewAndroidDeviceVerifier returns a new device.Verifier for Android devices
//
// Requires GOOGLE_APPLICATION_CREDENTIALS to be set
func NewAndroidDeviceVerifier(packageName string, minVersion int64) (device.Verifier, error) {
	ctx := context.Background()

	playIntegrityService, err := playintegrity.NewService(ctx)
	if err != nil {
		return nil, err
	}

	return &androidDeviceVerifier{
		playIntegrity: playIntegrityService,
		packageName:   packageName,
		minVersion:    minVersion,
	}, nil
}

// IsValid implements device.Verifier.IsValid
func (v *androidDeviceVerifier) IsValid(ctx context.Context, token string) (bool, string, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValid")
	defer tracer.End()

	isValid, reason, err := func() (bool, string, error) {
		resp, err := v.playIntegrity.V1.DecodeIntegrityToken(v.packageName, &playintegrity.DecodeIntegrityTokenRequest{
			IntegrityToken: token,
		}).Context(ctx).Do()
		if err != nil {
			return false, "", err
		}

		if resp.ServerResponse.HTTPStatusCode != http.StatusOK {
			return false, "", errors.Errorf("received http status %d", resp.ServerResponse.HTTPStatusCode)
		}

		requestedAt := time.UnixMilli(resp.TokenPayloadExternal.RequestDetails.TimestampMillis)
		if time.Since(requestedAt) > time.Minute {
			return false, "device token is too old", nil
		}

		if resp.TokenPayloadExternal.AppIntegrity.AppRecognitionVerdict != "PLAY_RECOGNIZED" {
			return false, fmt.Sprintf("app recognition verdict is %s", resp.TokenPayloadExternal.AppIntegrity.AppRecognitionVerdict), nil
		}

		if resp.TokenPayloadExternal.AccountDetails.AppLicensingVerdict != "LICENSED" {
			return false, fmt.Sprintf("app licensing verdict is %s", resp.TokenPayloadExternal.AccountDetails.AppLicensingVerdict), nil
		}

		if len(resp.TokenPayloadExternal.DeviceIntegrity.DeviceRecognitionVerdict) == 0 {
			return false, "no device recognition verdicts", nil
		}

		for _, deviceRecognitionVerdict := range resp.TokenPayloadExternal.DeviceIntegrity.DeviceRecognitionVerdict {
			switch deviceRecognitionVerdict {
			case "MEETS_VIRTUAL_INTEGRITY", "UNKNOWN":
				return false, fmt.Sprintf("device recognition verdict is %s", deviceRecognitionVerdict), nil
			}
		}

		if resp.TokenPayloadExternal.AppIntegrity.PackageName != v.packageName {
			return false, fmt.Sprintf("package name is is %s", resp.TokenPayloadExternal.AppIntegrity.PackageName), nil
		}

		if resp.TokenPayloadExternal.AppIntegrity.VersionCode < v.minVersion {
			return false, "minimum client version not met", nil
		}

		return true, "", nil
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return isValid, reason, err
}

// HasCreatedFreeAccount implements device.Verifier.HasCreatedFreeAccount
//
// todo: Currently impossible to implement without a stable device ID
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
// todo: Currently impossible to implement without a stable device ID
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
