package timelock_token

type TimelockState uint8

const (
	Unknown TimelockState = iota
	Unlocked
	WaitingForTimeout
	Locked
	Closed
)

func putTimelockAccountState(dst []byte, v TimelockState, offset *int) {
	putUint8(dst, uint8(v), offset)
}

func getTimelockAccountState(src []byte, dst *TimelockState, offset *int) {
	var v uint8
	getUint8(src, &v, offset)
	*dst = TimelockState(v)
}

func (s TimelockState) String() string {
	switch s {
	case Unknown:
		return "unknown"
	case Locked:
		return "locked"
	case WaitingForTimeout:
		return "waiting_for_timeout"
	case Unlocked:
		return "unlocked"
	case Closed:
		return "closed"
	default:
		return "unknown"
	}
}

type TimelockDataVersion uint8

const (
	UnknownDataVersion TimelockDataVersion = iota
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

type TimelockPdaVersion uint8

const (
	UnknownPdaVersion TimelockPdaVersion = iota
	PdaVersion1
)

func putTimelockAccountPdaVersion(dst []byte, v TimelockPdaVersion, offset *int) {
	putUint8(dst, uint8(v), offset)
}

func getTimelockAccountPdaVersion(src []byte, dst *TimelockPdaVersion, offset *int) {
	var v uint8
	getUint8(src, &v, offset)
	*dst = TimelockPdaVersion(v)
}
