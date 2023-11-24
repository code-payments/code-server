package timelock_token

type TimelockState uint8

const (
	StateUnknown TimelockState = iota
	StateUnlocked
	StateWaitingForTimeout
	StateLocked
	StateClosed
)

func putTimelockAccountState(dst []byte, v TimelockState, offset *int) {
	putUint8(dst, uint8(v), offset)
}

func getTimelockAccountState(src []byte, dst *TimelockState, offset *int) {
	var v uint8
	getUint8(src, &v, offset)
	*dst = TimelockState(v)
}

type TimelockDataVersion uint8

const (
	UnknownDataVersion TimelockDataVersion = iota
	DataVersionLegacy
	DataVersionClosed
	DataVersion1
)

func putTimelockAccountDataVersion(dst []byte, v TimelockDataVersion, offset *int) {
	putUint8(dst, uint8(v), offset)
}

func getTimelockAccountDataVersion(src []byte, dst *TimelockDataVersion, offset *int) {
	var v uint8
	getUint8(src, &v, offset)
	*dst = TimelockDataVersion(v)
}

func (s TimelockState) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateUnlocked:
		return "unlocked"
	case StateWaitingForTimeout:
		return "waiting_for_timeout"
	case StateLocked:
		return "locked"
	case StateClosed:
		return "closed"
	}

	return "unknown"
}
