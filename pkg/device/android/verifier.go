package android

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/api/playintegrity/v1"

	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/pkg/errors"
)

const (
	metricsStructName = "device.android.verifier"
)

type androidDeviceVerifier struct {
	playIntegrity *playintegrity.Service
	packageName   string
}

// NewAndroidDeviceVerifier returns a new device.Verifier for Android devices
//
// Requires GOOGLE_APPLICATION_CREDENTIALS to be set
func NewAndroidDeviceVerifier(packageName string) (device.Verifier, error) {
	ctx := context.Background()

	playintegrityService, err := playintegrity.NewService(ctx)
	if err != nil {
		return nil, err
	}

	return &androidDeviceVerifier{
		playIntegrity: playintegrityService,
		packageName:   packageName,
	}, nil
}

// IsValid implements device.Verifier.IsValid
func (v *androidDeviceVerifier) IsValid(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValid")
	defer tracer.End()

	isValid, err := func() (bool, error) {
		resp, err := v.playIntegrity.V1.DecodeIntegrityToken(v.packageName, &playintegrity.DecodeIntegrityTokenRequest{
			IntegrityToken: token,
		}).Context(ctx).Do()
		if err != nil {
			return false, err
		}

		if resp.ServerResponse.HTTPStatusCode != http.StatusOK {
			return false, errors.Errorf("received http status %d", resp.ServerResponse.HTTPStatusCode)
		}

		requestedAt := time.UnixMilli(resp.TokenPayloadExternal.RequestDetails.TimestampMillis)
		if time.Since(requestedAt) > time.Minute {
			return false, nil
		}

		if resp.TokenPayloadExternal.AppIntegrity.AppRecognitionVerdict != "PLAY_RECOGNIZED" {
			return false, nil
		}

		if resp.TokenPayloadExternal.AppIntegrity.PackageName != v.packageName {
			return false, nil
		}

		return true, nil
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
