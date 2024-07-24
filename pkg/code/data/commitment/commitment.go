package commitment

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
)

type State uint8

const (
	StateUnknown           State = iota
	StatePayingDestination       // No longer valid in the CVM
	StateReadyToOpen             // No longer valid in the CVM
	StateOpening                 // No longer valid in the CVM
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

	Address string

	Pool       string
	RecentRoot string

	Transcript  string
	Destination string
	Amount      uint64

	Intent   string
	ActionId uint32

	Owner string

	// Has the treasury been repaid for advancing Record.Amount to Record.Destination?
	// Not to be confused with payments being diverted to this commitment and then
	// being closed.
	TreasuryRepaid bool
	// The commitment where repayment for Record.Amount will be diverted to.
	RepaymentDivertedTo *string

	State State

	CreatedAt time.Time
}

func (r *Record) Validate() error {
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

	if len(r.Intent) == 0 {
		return errors.New("intent is required")
	}

	if len(r.Owner) == 0 {
		return errors.New("owner is required")
	}

	return nil
}

func (r *Record) Clone() *Record {
	return &Record{
		Id: r.Id,

		Address: r.Address,

		Pool:       r.Pool,
		RecentRoot: r.RecentRoot,

		Transcript:  r.Transcript,
		Destination: r.Destination,
		Amount:      r.Amount,

		Intent:   r.Intent,
		ActionId: r.ActionId,

		Owner: r.Owner,

		TreasuryRepaid:      r.TreasuryRepaid,
		RepaymentDivertedTo: pointer.StringCopy(r.RepaymentDivertedTo),

		State: r.State,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Address = r.Address

	dst.Pool = r.Pool
	dst.RecentRoot = r.RecentRoot

	dst.Transcript = r.Transcript
	dst.Destination = r.Destination
	dst.Amount = r.Amount

	dst.Intent = r.Intent
	dst.ActionId = r.ActionId

	dst.Owner = r.Owner

	dst.TreasuryRepaid = r.TreasuryRepaid
	dst.RepaymentDivertedTo = pointer.StringCopy(r.RepaymentDivertedTo)

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
