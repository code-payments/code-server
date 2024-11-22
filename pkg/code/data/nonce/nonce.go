package nonce

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/mr-tron/base58"
)

var (
	ErrNonceNotFound = errors.New("no records could be found")
	ErrInvalidNonce  = errors.New("invalid nonce")
)

type State uint8

const (
	StateUnknown   State = iota
	StateReleased        // The nonce is almost ready but we don't know its blockhash yet.
	StateAvailable       // The nonce is available to be used by a payment intent, subscription, or other nonce-related transaction.
	StateReserved        // The nonce is reserved by a payment intent, subscription, or other nonce-related transaction.
	StateInvalid         // The nonce account is invalid (e.g. insufficient funds, etc).
	StateClaimed         // The nonce is claimed for future use by a process (identified by Node ID).
)

// Purpose indicates the intended use purpose of the nonce. By partitioning nonce's by
// purpose, we help prevent various use cases from starving each other.
type Purpose uint8

const (
	PurposeUnknown Purpose = iota
	PurposeClientTransaction
	PurposeInternalServerProcess
	PurposeOnDemandTransaction
)

type Record struct {
	Id uint64

	Address   string
	Authority string
	Blockhash string
	Purpose   Purpose
	State     State

	// Contains the NodeId that transitioned the state into StateClaimed.
	//
	// Should be ignored if State != StateClaimed.
	ClaimNodeId string

	// The time at which StateClaimed is no longer valid, and the state should
	// be considered StateAvailable.
	//
	// Should be ignored if State != StateClaimed.
	ClaimExpiresAt time.Time

	Signature string
}

func (r *Record) GetPublicKey() (ed25519.PublicKey, error) {
	return base58.Decode(r.Address)
}

func (r *Record) IsAvailable() bool {
	if r.State == StateAvailable {
		return true
	}
	if r.State != StateClaimed {
		return false
	}

	return time.Now().After(r.ClaimExpiresAt)
}

func (r *Record) Clone() Record {
	return Record{
		Id:             r.Id,
		Address:        r.Address,
		Authority:      r.Authority,
		Blockhash:      r.Blockhash,
		Purpose:        r.Purpose,
		State:          r.State,
		ClaimNodeId:    r.ClaimNodeId,
		ClaimExpiresAt: r.ClaimExpiresAt,
		Signature:      r.Signature,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.Address = r.Address
	dst.Authority = r.Authority
	dst.Blockhash = r.Blockhash
	dst.Purpose = r.Purpose
	dst.State = r.State
	dst.ClaimNodeId = r.ClaimNodeId
	dst.ClaimExpiresAt = r.ClaimExpiresAt
	dst.Signature = r.Signature
}

func (r *Record) Validate() error {
	if len(r.Address) == 0 {
		return errors.New("nonce account address is required")
	}

	if len(r.Authority) == 0 {
		return errors.New("authority address is required")
	}

	if r.Purpose == PurposeUnknown {
		return errors.New("nonce purpose must be set")
	}

	if r.State == StateClaimed {
		if r.ClaimNodeId == "" {
			return errors.New("missing claim node id")
		}
		if r.ClaimExpiresAt == (time.Time{}) || r.ClaimExpiresAt.IsZero() {
			return errors.New("missing claim expiry date")
		}
	}

	return nil
}

func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateReleased:
		return "released"
	case StateAvailable:
		return "available"
	case StateReserved:
		return "reserved"
	case StateInvalid:
		return "invalid"
	case StateClaimed:
		return "claimed"
	}

	return "unknown"
}

func (p Purpose) String() string {
	switch p {
	case PurposeUnknown:
		return "unknown"
	case PurposeClientTransaction:
		return "client_transaction"
	case PurposeInternalServerProcess:
		return "internal_server_process"
	case PurposeOnDemandTransaction:
		return "on_demand_transaction"
	}

	return "unknown"
}
