package nonce

import (
	"crypto/ed25519"
	"errors"

	"github.com/mr-tron/base58"
)

type Environment uint8

const (
	EnvironmentUnknown Environment = iota
	EnvironmentSolana              // Environment instance is the cluster name (ie. mainnet, devnet, testnet, etc)
	EnvironmentCvm                 // Environment instance is the VM public key
)

const (
	EnvironmentInstanceSolanaMainnet = "mainnet"
	EnvironmentInstanceSolanaDevnet  = "devnet"
	EnvironmentInstanceSolanaTestnet = "testnet"
)

var (
	ErrNonceNotFound = errors.New("no records could be found")
	ErrInvalidNonce  = errors.New("invalid nonce")
)

type State uint8

const (
	StateUnknown   State = iota
	StateReleased        // The nonce is almost ready but we don't know its blockhash yet.
	StateAvailable       // The nonce is available to be used by a payment intent, subscription, or other nonce-related transaction/instruction.
	StateReserved        // The nonce is reserved by a payment intent, subscription, or other nonce-related transaction/instruction.
	StateInvalid         // The nonce account is invalid (e.g. insufficient funds, etc).
)

// Split nonce pool across different use cases. This has an added benefit of:
//   - Solving for race conditions without distributed locks.
//   - Avoiding different use cases from starving each other and ending up in a
//     deadlocked state. Concretely, it would be really bad if clients could starve
//     internal processes from creating transactions that would allow us to progress
//     and submit existing transactions.
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

	Environment         Environment
	EnvironmentInstance string

	Purpose Purpose
	State   State

	Signature string
}

func (r *Record) GetPublicKey() (ed25519.PublicKey, error) {
	return base58.Decode(r.Address)
}

func (r *Record) Clone() Record {
	return Record{
		Id:                  r.Id,
		Address:             r.Address,
		Authority:           r.Authority,
		Blockhash:           r.Blockhash,
		Environment:         r.Environment,
		EnvironmentInstance: r.EnvironmentInstance,
		Purpose:             r.Purpose,
		State:               r.State,
		Signature:           r.Signature,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.Address = r.Address
	dst.Authority = r.Authority
	dst.Blockhash = r.Blockhash
	dst.Environment = r.Environment
	dst.EnvironmentInstance = r.EnvironmentInstance
	dst.Purpose = r.Purpose
	dst.State = r.State
	dst.Signature = r.Signature
}

func (v *Record) Validate() error {
	if len(v.Address) == 0 {
		return errors.New("nonce account address is required")
	}

	if len(v.Authority) == 0 {
		return errors.New("authority address is required")
	}

	if v.Environment == EnvironmentUnknown {
		return errors.New("nonce environment must be set")
	}

	if len(v.EnvironmentInstance) == 0 {
		return errors.New("nonce environment instance must be set")
	}

	if v.Purpose == PurposeUnknown {
		return errors.New("nonce purpose must be set")
	}

	return nil
}

func (e Environment) String() string {
	switch e {
	case EnvironmentUnknown:
		return "unknown"
	case EnvironmentSolana:
		return "solana"
	case EnvironmentCvm:
		return "cvm"
	}
	return "unknown"
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
