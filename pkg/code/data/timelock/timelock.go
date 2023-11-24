package timelock

import (
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	timelock_token_legacy "github.com/code-payments/code-server/pkg/solana/timelock/legacy_2022"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

var (
	ErrTimelockNotFound   = errors.New("no records could be found")
	ErrInvalidTimelock    = errors.New("invalid timelock")
	ErrStaleTimelockState = errors.New("timelock state is stale")
)

// Based off of https://github.com/code-payments/code-program-library/blob/main/timelock-token/programs/timelock-token/src/state.rs
//
// This record supports both the legacy 2022 (pre-privacy) and v1 versions of
// the timelock program, since they're easily interchangeable. All legacy fields
// will be converted accordingly, or dropped if never used.
type Record struct {
	Id uint64

	DataVersion timelock_token_v1.TimelockDataVersion

	Address string
	Bump    uint8

	VaultAddress string
	VaultBump    uint8
	VaultOwner   string
	VaultState   timelock_token_v1.TimelockState

	TimeAuthority  string
	CloseAuthority string

	NumDaysLocked uint8
	UnlockAt      *uint64

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

func (r *Record) UpdateFromV1ProgramAccount(data *timelock_token_v1.TimelockAccount, block uint64) error {
	// Avoid updates looking backwards in blockchain history

	if block <= r.Block {
		return ErrStaleTimelockState
	}

	// Check the expected data version. If we encounter a closed version, simply
	// update the record's data version. Expect all other data to be garbage, so
	// don't include it. Ideally this should never happen, but we need to be
	// extra safe in the event of an unknown attack vector. This can directly
	// affect our ability to manage an account's funds.

	if data.DataVersion == timelock_token_v1.DataVersionClosed {
		r.DataVersion = timelock_token_v1.DataVersionClosed
		r.Block = block
		return nil
	}

	if data.DataVersion != timelock_token_v1.DataVersion1 {
		return errors.New("timelock data version must be 1")
	}

	// It's now safe to update the record

	var unlockAt *uint64
	if data.UnlockAt != nil {
		value := *data.UnlockAt
		unlockAt = &value
	}

	// These 2 fields should never change under ideal circumstances, but we need
	// to be extra safe in the event of an unknown attack vector. These directly
	// affect our ability to manage an account's funds.
	r.TimeAuthority = base58.Encode(data.TimeAuthority)
	r.CloseAuthority = base58.Encode(data.CloseAuthority)

	r.VaultState = data.VaultState
	r.UnlockAt = unlockAt

	r.Block = block

	return nil
}

func (r *Record) UpdateFromLegacy2022ProgramAccount(data *timelock_token_legacy.TimelockAccount, block uint64) error {
	// Avod updates looking backwards in blockchain history

	if block <= r.Block {
		return ErrStaleTimelockState
	}

	// Check the expected data version

	// Handle init_offset similarly to how we handle DataVersionClosed in the v1
	// update method.
	if data.InitOffset != 0 {
		r.DataVersion = timelock_token_v1.DataVersionClosed
		r.Block = block
		return nil
	}

	if data.DataVersion != timelock_token_legacy.TimelockDataVersion(timelock_token_v1.DataVersionLegacy) {
		return errors.New("timelock data version must be legacy")
	}

	// It's now safe to update the record

	var unlockAt *uint64
	if data.UnlockAt != nil {
		value := *data.UnlockAt
		unlockAt = &value
	}

	// These 2 fields should never change under ideal circumstances, but we need
	// to be extra safe in the event of an unknown attack vector. These directly
	// affect our ability to manage an account's funds.
	r.TimeAuthority = base58.Encode(data.TimeAuthority)
	r.CloseAuthority = base58.Encode(data.CloseAuthority)

	r.VaultState = timelock_token_v1.TimelockState(data.VaultState)
	r.UnlockAt = unlockAt

	r.Block = block

	return nil
}

func (r *Record) Clone() *Record {
	var unlockAt *uint64
	if r.UnlockAt != nil {
		value := *r.UnlockAt
		unlockAt = &value
	}

	return &Record{
		Id: r.Id,

		DataVersion: r.DataVersion,

		Address: r.Address,
		Bump:    r.Bump,

		VaultAddress: r.VaultAddress,
		VaultBump:    r.VaultBump,
		VaultOwner:   r.VaultOwner,
		VaultState:   r.VaultState,

		TimeAuthority:  r.TimeAuthority,
		CloseAuthority: r.CloseAuthority,

		NumDaysLocked: r.NumDaysLocked,
		UnlockAt:      unlockAt,

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

	dst.DataVersion = r.DataVersion

	dst.Address = r.Address
	dst.Bump = r.Bump

	dst.VaultAddress = r.VaultAddress
	dst.VaultBump = r.VaultBump
	dst.VaultOwner = r.VaultOwner
	dst.VaultState = r.VaultState

	dst.TimeAuthority = r.TimeAuthority
	dst.CloseAuthority = r.CloseAuthority

	dst.NumDaysLocked = r.NumDaysLocked
	dst.UnlockAt = unlockAt

	dst.Block = r.Block

	dst.LastUpdatedAt = r.LastUpdatedAt
}

func (r *Record) Validate() error {
	if r == nil {
		return errors.New("record is nil")
	}

	switch r.DataVersion {
	case timelock_token_v1.DataVersionLegacy,
		timelock_token_v1.DataVersion1,
		timelock_token_v1.DataVersionClosed:
	default:
		return errors.New("invalid timelock data version")
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

	if len(r.TimeAuthority) == 0 {
		return errors.New("time authority is required")
	}

	if len(r.CloseAuthority) == 0 {
		return errors.New("close authority is required")
	}

	if r.NumDaysLocked != timelock_token_v1.DefaultNumDaysLocked {
		return errors.Errorf("num days locked must be %d days", timelock_token_v1.DefaultNumDaysLocked)
	}

	return nil
}
