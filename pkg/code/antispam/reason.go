package antispam

type Reason int

const (
	ReasonUnspecified Reason = iota
	ReasonUnsupportedCountry
	ReasonUnsupportedDevice
	ReasonTooManyFreeAccountsForPhoneNumber
	ReasonTooManyFreeAccountsForDevice
)
