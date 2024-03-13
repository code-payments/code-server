package fcm

import (
	"context"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/push"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	metricsStructName = "push.fcm.provider"
)

type provider struct {
	client *messaging.Client
}

// NewPushProvider returns a new push.Provider backed by FCM
//
// Requires GOOGLE_APPLICATION_CREDENTIALS to be set:
// https://firebase.google.com/docs/admin/setup#initialize-sdk
func NewPushProvider() (push.Provider, error) {
	firebaseApp, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "error initializing firebase app")
	}

	messagingClient, err := firebaseApp.Messaging(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "error initializing firebase messaging client")
	}

	return &provider{
		client: messagingClient,
	}, nil
}

// IsValidPushToken implements push.Provider.IsValidPushToken
func (p *provider) IsValidPushToken(ctx context.Context, pushToken string) (bool, error) {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "IsValidPushToken").End()

	var result bool
	err := retrier(func() error {
		_, err := p.client.SendDryRun(ctx, &messaging.Message{
			Token: pushToken,
			Notification: &messaging.Notification{
				Title: "test",
			},
		})

		// https://firebase.google.com/docs/cloud-messaging/manage-tokens#detect-invalid-token-responses-from-the-fcm-backend
		if messaging.IsInvalidArgument(err) || messaging.IsUnregistered(err) || messaging.IsSenderIDMismatch(err) {
			return nil
		} else if err != nil {
			return err
		}

		result = true
		return nil
	})

	if err != nil {
		return false, errors.Wrap(err, "error sending dry run message")
	}
	return result, nil
}

// SendPush implements push.Provider.SendPush
func (p *provider) SendPush(ctx context.Context, pushToken, title, body string) error {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "SendPush").End()

	return retrier(func() error {
		_, err := p.client.Send(ctx, &messaging.Message{
			Token: pushToken,
			Notification: &messaging.Notification{
				Title: title,
				Body:  body,
			},
		})
		return err
	})
}

// SendMutableAPNSPush implements push.Provider.SendMutableAPNSPush
func (p *provider) SendMutableAPNSPush(
	ctx context.Context,
	pushToken,
	titleKey, category, threadId string,
	kvs map[string]string,
) error {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "SendMutableAPNSPush").End()

	_, err := p.client.Send(ctx, &messaging.Message{
		Token: pushToken,
		Data:  kvs,
		APNS: &messaging.APNSConfig{
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Alert: &messaging.ApsAlert{
						TitleLocKey: titleKey,
						Body:        "...",
					},
					Category:       category,
					ThreadID:       threadId,
					MutableContent: true,
				},
			},
		},
	})
	return err
}

// SendDataPush implements push.Provider.SendDataPush
func (p *provider) SendDataPush(ctx context.Context, pushToken string, kvs map[string]string) error {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "SendDataPush").End()

	return retrier(func() error {
		_, err := p.client.Send(ctx, &messaging.Message{
			Token: pushToken,
			Data:  kvs,
		})
		return err
	})
}

// SetAPNSBadgeCount implements push.Provider.SetAPNSBadgeCount
func (p *provider) SetAPNSBadgeCount(ctx context.Context, pushToken string, count int) error {
	defer metrics.TraceMethodCall(ctx, metricsStructName, "SetAPNSBadgeCount").End()

	return retrier(func() error {
		_, err := p.client.Send(ctx, &messaging.Message{
			Token: pushToken,
			APNS: &messaging.APNSConfig{
				Payload: &messaging.APNSPayload{
					Aps: &messaging.Aps{
						Badge: &count,
					},
				},
			},
		})
		return err
	})
}

// retrier is a common retry strategy for FCM calls
func retrier(action retry.Action) error {
	_, err := retry.Retry(
		action,
		retry.Limit(3),
		func(attempts uint, err error) bool {
			return messaging.IsUnavailable(err) || messaging.IsInternal(err)
		},
		retry.Backoff(backoff.BinaryExponential(250*time.Millisecond), time.Second),
	)
	return err
}
