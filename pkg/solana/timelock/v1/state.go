package timelock_token

type TimelockState uint8

const (
	StateUnknown TimelockState = iota
	StateUnlocked
	StateWaitingForTimeout
	StateLocked
	StateClosed
)

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
