package timelock_token

type TimeLockTokenError uint32

const (
	// Invalid time-lock state for this instruction
	ErrInvalidTimeLockState TimeLockTokenError = iota + 0x1770

	// Invalid time-lock duration provided
	ErrInvalidTimeLockDuration

	// Invalid vault account
	ErrInvalidVaultAccount

	// The time-lock period has not yet been reached
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

	// Invalid dust burn
	ErrInvalidDustBurn

	// Invalid token mint
	ErrInvalidTokenMint
)
