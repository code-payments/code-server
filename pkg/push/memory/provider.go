package memory

import (
	"context"
	"errors"

	"github.com/code-payments/code-server/pkg/push"
)

const (
	// This values will pass IsValidPushToken
	ValidAndroidPushToken = "test_android_push_token"
	ValidApplePushToken   = "test_apns_push_token"

	// This value will fail IsValidPushToken
	InvalidPushToken = "invalid"
)

type provider struct {
}

// NewPushProvider returns a new in memory push.Provider
func NewPushProvider() push.Provider {
	return &provider{}
}

// IsValidPushToken implements push.Provider.IsValidPushToken
func (p *provider) IsValidPushToken(_ context.Context, pushToken string) (bool, error) {
	return pushToken == ValidAndroidPushToken || pushToken == ValidApplePushToken, nil
}

// SendPush implements push.Provider.SendPush
func (p *provider) SendPush(ctx context.Context, pushToken, title, body string) error {
	return simulateSendingPush(pushToken)
}

// SendLocalizedAPNSPush implements push.Provider.SendLocalizedAPNSPush
func (p *provider) SendLocalizedAPNSPush(ctx context.Context, pushToken, titleKey, bodyKey string, bodyArgs ...string) error {
	return simulateSendingPush(pushToken)
}

// SendLocalizedAndroidPush implements push.Provider.SendLocalizedAndroidPush
func (p *provider) SendLocalizedAndroidPush(ctx context.Context, pushToken, titleKey, bodyKey string, bodyArgs ...string) error {
	return simulateSendingPush(pushToken)
}

// SendMutableAPNSPush implements push.Provider.SendMutableAPNSPush
func (p *provider) SendMutableAPNSPush(ctx context.Context, pushToken, titleKey string, kvs map[string]string) error {
	return simulateSendingPush(pushToken)
}

// SendDataPush implements push.Provider.SendDataPush
func (p *provider) SendDataPush(ctx context.Context, pushToken string, kvs map[string]string) error {
	return simulateSendingPush(pushToken)
}

// SetAPNSBadgeCount implements push.Provider.SetAPNSBadgeCount
func (p *provider) SetAPNSBadgeCount(ctx context.Context, pushToken string, count int) error {
	return simulateSendingPush(pushToken)
}

func simulateSendingPush(pushToken string) error {
	if pushToken == ValidAndroidPushToken || pushToken == ValidApplePushToken {
		return nil
	}

	return errors.New("push token is invalid")
}
