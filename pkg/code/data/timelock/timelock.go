package timelock

import (
	"time"

	"github.com/pkg/errors"

	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

var (
	ErrTimelockNotFound   = errors.New("no records could be found")
	ErrInvalidTimelock    = errors.New("invalid timelock")
	ErrStaleTimelockState = errors.New("timelock state is stale")
)

// Time/close authorities and lock duration are configured at the VM level
//
// todo: Assumes a single VM.
type Record struct {
	Id uint64

	Address string
	Bump    uint8

	VaultAddress string
	VaultBump    uint8
	VaultOwner   string
	VaultState   timelock_token_v1.TimelockState // Uses the original Timelock account state since the CVM only defines enum states for unlock

	DepositPdaAddress string
	DepositPdaBump    uint8

	SwapPdaAddress string
	SwapPdaBump    uint8

	UnlockAt *uint64

	Block uint64

	LastUpdatedAt time.Time
}

func (r *Record) IsLocked() bool {
	// Special case where the account is being actively created and we have no
	// known state from the blockchain. Newly initialized accounts are guaranteed
	// to be in the locked state by the timelock program.
	if r.VaultState == timelock_token_v1.StateUnknown && r.Block == 0 {
		return true
	}

	return r.VaultState == timelock_token_v1.StateLocked
}

func (r *Record) IsClosed() bool {
	return r.VaultState == timelock_token_v1.StateClosed
}

func (r *Record) ExistsOnBlockchain() bool {
	return r.VaultState != timelock_token_v1.StateUnknown && r.VaultState != timelock_token_v1.StateClosed
}

func (r *Record) Clone() *Record {
	var unlockAt *uint64
	if r.UnlockAt != nil {
		value := *r.UnlockAt
		unlockAt = &value
	}

	return &Record{
		Id: r.Id,

		Address: r.Address,
		Bump:    r.Bump,

		VaultAddress: r.VaultAddress,
		VaultBump:    r.VaultBump,
		VaultOwner:   r.VaultOwner,
		VaultState:   r.VaultState,

		DepositPdaAddress: r.DepositPdaAddress,
		DepositPdaBump:    r.DepositPdaBump,

		SwapPdaAddress: r.SwapPdaAddress,
		SwapPdaBump:    r.SwapPdaBump,

		UnlockAt: unlockAt,

		Block: r.Block,

		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	var unlockAt *uint64
	if r.UnlockAt != nil {
		value := *r.UnlockAt
		unlockAt = &value
	}

	dst.Id = r.Id

	dst.Address = r.Address
	dst.Bump = r.Bump

	dst.VaultAddress = r.VaultAddress
	dst.VaultBump = r.VaultBump
	dst.VaultOwner = r.VaultOwner
	dst.VaultState = r.VaultState

	dst.DepositPdaAddress = r.DepositPdaAddress
	dst.DepositPdaBump = r.DepositPdaBump

	dst.SwapPdaAddress = r.SwapPdaAddress
	dst.SwapPdaBump = r.SwapPdaBump

	dst.UnlockAt = unlockAt

	dst.Block = r.Block

	dst.LastUpdatedAt = r.LastUpdatedAt
}

func (r *Record) Validate() error {
	if r == nil {
		return errors.New("record is nil")
	}

	if len(r.Address) == 0 {
		return errors.New("state address is required")
	}

	if len(r.VaultAddress) == 0 {
		return errors.New("vault address is required")
	}

	if len(r.VaultOwner) == 0 {
		return errors.New("vault owner is required")
	}

	if len(r.DepositPdaAddress) == 0 {
		return errors.New("deposit pda address is required")
	}

	if len(r.SwapPdaAddress) == 0 {
		return errors.New("swap pda address is required")
	}

	return nil
}
