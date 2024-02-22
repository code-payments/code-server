package push

import (
	"context"
)

type Provider interface {
	// IsValidPushToken validates whether a push token is valid
	IsValidPushToken(ctx context.Context, pushToken string) (bool, error)

	// SendPush sends a basic push notication with a title and body
	SendPush(ctx context.Context, pushToken, title, body string) error

	// SendMutableAPNSPush sends a push over APNS with a text body that's mutable
	// on the client using custom key value pairs
	SendMutableAPNSPush(ctx context.Context, pushToken, titleKey string, kvs map[string]string) error

	// SendDataPush sends a data push
	SendDataPush(ctx context.Context, pushToken string, kvs map[string]string) error

	// SetAPNSBadgeCount sets the badge count on the iOS app icon
	SetAPNSBadgeCount(ctx context.Context, pushToken string, count int) error
}
