package client

import "strings"

type DeviceType uint8

const (
	DeviceTypeUnknown DeviceType = iota
	DeviceTypeIOS
	DeviceTypeAndroid
)

func (t DeviceType) String() string {
	switch t {
	case DeviceTypeUnknown:
		return "Unknown"
	case DeviceTypeIOS:
		return "iOS"
	case DeviceTypeAndroid:
		return "Android"
	}
	return "Unknown"
}

func (t DeviceType) IsMobile() bool {
	switch t {
	case DeviceTypeIOS, DeviceTypeAndroid:
		return true
	}
	return false
}

func deviceTypeFromString(value string) DeviceType {
	switch strings.ToLower(value) {
	case "ios":
		return DeviceTypeIOS
	case "android":
		return DeviceTypeAndroid
	}
	return DeviceTypeUnknown
}
