package token

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana/binary"
)

type AccountState byte

const (
	AccountStateUninitialized AccountState = iota
	AccountStateInitialized
	AccountStateFrozen
)

// Reference: https://github.com/solana-labs/solana-program-library/blob/11b1e3eefdd4e523768d63f7c70a7aa391ea0d02/token/program/src/state.rs#L125
const AccountSize = 165

// Reference: https://github.com/solana-labs/solana-program-library/blob/8944f428fe693c3a4226bf766a79be9c75e8e520/token/program/src/state.rs#L214
const MultisigAccountSize = 355

const optionSize = 4

type Account struct {
	// The mint associated with this account
	Mint ed25519.PublicKey
	// The owner of this account.
	Owner ed25519.PublicKey
	// The amount of tokens this account holds.
	Amount uint64
	// If set, then the 'DelegatedAmount' represents the amount
	// authorized by the delegate.
	Delegate ed25519.PublicKey
	/// The account's state
	State AccountState
	// If set, this is a native token, and the value logs the rent-exempt reserve. An Account
	// is required to be rent-exempt, so the value is used by the Processor to ensure that wrapped
	// SOL accounts do not drop below this threshold.
	IsNative *uint64
	// The amount delegated
	DelegatedAmount uint64
	// Optional authority to close the account.
	CloseAuthority ed25519.PublicKey
}

func (a *Account) Marshal() []byte {
	b := make([]byte, AccountSize)

	var offset int
	binary.PutKey32(b, a.Mint, &offset)
	binary.PutKey32(b[offset:], a.Owner, &offset)
	binary.PutUint64(b[offset:], a.Amount, &offset)
	binary.PutOptionalKey32(b[offset:], a.Delegate, &offset, optionSize)
	b[offset] = byte(a.State)
	offset++
	binary.PutOptionalUint64(b[offset:], a.IsNative, &offset, optionSize)
	binary.PutUint64(b[offset:], a.DelegatedAmount, &offset)
	binary.PutOptionalKey32(b[offset:], a.CloseAuthority, &offset, optionSize)

	return b
}

func (a *Account) Unmarshal(b []byte) bool {
	if len(b) != AccountSize {
		return false
	}

	var offset int
	binary.GetKey32(b, &a.Mint, &offset)
	binary.GetKey32(b[offset:], &a.Owner, &offset)
	binary.GetUint64(b[offset:], &a.Amount, &offset)
	binary.GetOptionalKey32(b[offset:], &a.Delegate, &offset, optionSize)
	a.State = AccountState(b[offset])
	offset++
	binary.GetOptionalUint64(b[offset:], &a.IsNative, &offset, optionSize)
	binary.GetUint64(b[offset:], &a.DelegatedAmount, &offset)
	binary.GetOptionalKey32(b[offset:], &a.CloseAuthority, &offset, optionSize)

	return true
}
