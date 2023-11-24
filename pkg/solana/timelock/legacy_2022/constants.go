package timelock_token

import "time"

const (
	// Need to be very careful changing this value, as it's used in the state
	// address PDA.
	DefaultUnlockDuration = uint64(3 * 7 * 24 * time.Hour / time.Second) // 3 weeks in seconds
)
