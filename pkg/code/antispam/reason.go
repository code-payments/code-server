package antispam

type Reason int

const (
	ReasonUnspecified Reason = iota
	ReasonUnsupportedCountry
	ReasonUnsupportedDevice
	ReasonTooManyFreeAccountsForDevice
)
