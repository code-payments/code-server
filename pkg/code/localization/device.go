package localization

import (
	"context"
	"strings"

	"github.com/code-payments/code-server/pkg/grpc/client"
)

// GetLocalizationKeyForUserAgent gets a localization key in the format for the device
// as provided in the user agent header. If an unknown device type, or the user- gent
// header isn't available, then the original key is returned.
func GetLocalizationKeyForUserAgent(ctx context.Context, key string) string {
	userAgent, err := client.GetUserAgent(ctx)
	if err != nil {
		return key
	}

	switch userAgent.DeviceType {
	case client.DeviceTypeIOS:
		return GetIosLocalizationKey(key)
	case client.DeviceTypeAndroid:
		return GetAndroidLocalizationKey(key)
	}

	return key
}

// GetIosLocalizationKey gets a localization string in the iOS format
func GetIosLocalizationKey(key string) string {
	return strings.Replace(key, "_", ".", -1)
}

// GetAndroidLocalizationKey gets a localization string in the Android format
func GetAndroidLocalizationKey(key string) string {
	return strings.Replace(key, ".", "_", -1)
}
