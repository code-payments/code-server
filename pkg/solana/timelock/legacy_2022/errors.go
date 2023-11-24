package timelock_token

type TimelockTokenError uint32

const (
	// Invalid timelock state for this instruction
	ErrInvalidTimelockState TimelockTokenError = iota + 0x1770

	// Invalid timelock duration provided
	ErrInvalidTimelockDuration

	// Invalid vault account
	ErrInvalidVaultAccount

	// The timelock period has not yet been reached
	ErrInsufficientTimeElapsed

	// Insufficient vault funds
	ErrInsufficientVaultBalance

	// Invalid time authority
	ErrInvalidTimeAuthority

	// Invalid vault owner
	ErrInvalidVaultOwner

	// Invalid close authority
	ErrInvalidCloseAuthority

	// Invalid token balance. Token balance must be zero.
	ErrNonZeroTokenBalance
)
