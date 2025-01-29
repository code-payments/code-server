package cvm

type TimelockState uint8

const (
	TimelockStateUnknown TimelockState = iota
	TimelockStateUnlocked
	TimelockStateWaitingForTimeout
)

func getTimelockState(src []byte, dst *TimelockState, offset *int) {
	*dst = TimelockState(src[*offset])
	*offset += 1
}

func (s TimelockState) String() string {
	switch s {
	case TimelockStateUnlocked:
		return "unlocked"
	case TimelockStateWaitingForTimeout:
		return "waiting_for_timeout"
	}
	return "unknown"
}
