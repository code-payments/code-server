package android

import (
	"context"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/appcheck"
	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "device.android.verifier"
)

type androidDeviceVerifier struct {
	projectId string
	appId     string
	client    *appcheck.Client
}

// NewAndroidDeviceVerifier returns a new device.Verifier for Android devices
//
// Requires GOOGLE_APPLICATION_CREDENTIALS to be set:
// https://firebase.google.com/docs/admin/setup#initialize-sdk
func NewAndroidDeviceVerifier(
	projectId, appId string,
) (device.Verifier, error) {
	firebaseApp, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		return nil, err
	}

	appCheckClient, err := firebaseApp.AppCheck(context.Background())
	if err != nil {
		return nil, err
	}

	return &androidDeviceVerifier{
		projectId: projectId,
		appId:     appId,
		client:    appCheckClient,
	}, nil
}

// IsValid implements device.Verifier.IsValid
func (v *androidDeviceVerifier) IsValid(ctx context.Context, token string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "IsValid")
	defer tracer.End()

	isValid, err := func() (bool, error) {
		decodedToken, err := v.client.VerifyToken(token)
		if err != nil {
			return false, nil
		}

		// Ensure the token is for the official project
		var foundProjectId bool
		for _, audience := range decodedToken.Audience {
			if audience == "projects/"+v.projectId {
				foundProjectId = true
			}
		}
		if !foundProjectId {
			return false, nil
		}

		// Ensure the token is for the official application
		if decodedToken.AppID != v.appId {
			return false, nil
		}

		// Require very fresh tokens
		if time.Since(decodedToken.IssuedAt) > time.Minute {
			return false, nil
		}

		// todo: Consume the token to better avoid replay attacks when available
		//       https://firebase.google.com/docs/app-check/custom-resource-backend#replay-protection

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
