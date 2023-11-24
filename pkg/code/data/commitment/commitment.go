package commitment

import (
	"errors"
	"time"

	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
)

type State uint8

const (
	StateUnknown State = iota
	StatePayingDestination
	StateReadyToOpen
	StateOpening
	StateOpen
	StateClosing
	StateClosed

	// Theoretical states that don't have any logic right now, but will be needed
	// later for GC.
	StateReadyToRemoveFromMerkleTree
	StateRemovedFromMerkleTree
)

type Record struct {
	Id uint64

	DataVersion splitter_token.DataVersion

	Address string
	Bump    uint8

	Pool       string
	PoolBump   uint8
	RecentRoot string
	Transcript string

	Destination string
	Amount      uint64

	Vault     string
	VaultBump uint8

	Intent   string
	ActionId uint32

	Owner string

	// Has the treasury been repaid for advancing Record.Amount to Record.Destination?
	// Not to be confused with payments being diverted to this commitment and then
	// being closed.
	TreasuryRepaid bool
	// The commitment vault where repayment for Record.Amount will be diverted to.
	RepaymentDivertedTo *string

	State State

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if r.DataVersion != splitter_token.DataVersion1 {
		return errors.New("commitment data version must be 1")
	}

	if len(r.Address) == 0 {
		return errors.New("address is required")
	}

	if len(r.Pool) == 0 {
		return errors.New("pool is required")
	}

	if len(r.RecentRoot) == 0 {
		return errors.New("recent root is required")
	}

	if len(r.Transcript) == 0 {
		return errors.New("transcript is required")
	}

	if len(r.Destination) == 0 {
		return errors.New("destination is required")
	}

	if r.Amount == 0 {
		return errors.New("settlement amount must be positive")
	}

	if len(r.Vault) == 0 {
		return errors.New("vault is required")
	}

	if len(r.Intent) == 0 {
		return errors.New("intent is required")
	}

	if len(r.Owner) == 0 {
		return errors.New("owner is required")
	}

	return nil
}

func (r *Record) Clone() *Record {
	var repaymentDivertedTo *string
	if r.RepaymentDivertedTo != nil {
		value := *r.RepaymentDivertedTo
		repaymentDivertedTo = &value
	}

	return &Record{
		Id: r.Id,

		DataVersion: r.DataVersion,

		Address: r.Address,
		Bump:    r.Bump,

		Pool:       r.Pool,
		PoolBump:   r.PoolBump,
		RecentRoot: r.RecentRoot,
		Transcript: r.Transcript,

		Destination: r.Destination,
		Amount:      r.Amount,

		Vault:     r.Vault,
		VaultBump: r.VaultBump,

		Intent:   r.Intent,
		ActionId: r.ActionId,

		Owner: r.Owner,

		TreasuryRepaid:      r.TreasuryRepaid,
		RepaymentDivertedTo: repaymentDivertedTo,

		State: r.State,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.DataVersion = r.DataVersion

	dst.Address = r.Address
	dst.Bump = r.Bump

	dst.Pool = r.Pool
	dst.PoolBump = r.PoolBump
	dst.RecentRoot = r.RecentRoot
	dst.Transcript = r.Transcript

	dst.Destination = r.Destination
	dst.Amount = r.Amount

	dst.Vault = r.Vault
	dst.VaultBump = r.VaultBump

	dst.Intent = r.Intent
	dst.ActionId = r.ActionId

	dst.Owner = r.Owner

	dst.TreasuryRepaid = r.TreasuryRepaid
	dst.RepaymentDivertedTo = r.RepaymentDivertedTo

	dst.State = r.State

	dst.CreatedAt = r.CreatedAt
}

func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StatePayingDestination:
		return "paying_destination"
	case StateReadyToOpen:
		return "ready_to_open"
	case StateOpening:
		return "opening"
	case StateOpen:
		return "open"
	case StateClosing:
		return "closing"
	case StateClosed:
		return "closed"
	case StateReadyToRemoveFromMerkleTree:
		return "ready_to_remove_from_merkle_tree"
	case StateRemovedFromMerkleTree:
		return "removed_from_merkle_tree"
	}
	return "unknown"
}
