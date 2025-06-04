package solana

import (
	"bytes"
	"crypto/ed25519"
	"errors"
)

var (
	ErrIncorrectProgram     = errors.New("incorrect program")
	ErrIncorrectInstruction = errors.New("incorrect instruction")
)

// AccountMeta represents the account information required
// for building transactions.
type AccountMeta struct {
	PublicKey  ed25519.PublicKey
	IsSigner   bool
	IsWritable bool
	isPayer    bool
	isProgram  bool
}

// NewAccountMeta creates a new AccountMeta representing a writable
// account.
func NewAccountMeta(pub ed25519.PublicKey, isSigner bool) AccountMeta {
	return AccountMeta{
		PublicKey:  pub,
		IsSigner:   isSigner,
		IsWritable: true,
	}
}

// NewAccountMeta creates a new AccountMeta representing a readonly
// account.
func NewReadonlyAccountMeta(pub ed25519.PublicKey, isSigner bool) AccountMeta {
	return AccountMeta{
		PublicKey:  pub,
		IsSigner:   isSigner,
		IsWritable: false,
	}
}

// SortableAccountMeta is a sortable []AccountMeta based on the solana transaction
// account sorting rules.
//
// Reference: https://docs.solana.com/transaction#account-addresses-format
type SortableAccountMeta []AccountMeta

// Len is the number of elements in the collection.
func (s SortableAccountMeta) Len() int {
	return len(s)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s SortableAccountMeta) Less(i int, j int) bool {
	if s[i].isPayer != s[j].isPayer {
		return s[i].isPayer
	}
	if s[i].isProgram != s[j].isProgram {
		return !s[i].isProgram
	}

	if s[i].IsSigner != s[j].IsSigner {
		return s[i].IsSigner
	}
	if s[i].IsWritable != s[j].IsWritable {
		return s[i].IsWritable
	}

	return bytes.Compare(s[i].PublicKey, s[j].PublicKey) < 0
}

// Swap swaps the elements with indexes i and j.
func (s SortableAccountMeta) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

// Instruction represents a transaction instruction.
type Instruction struct {
	Program  ed25519.PublicKey
	Accounts []AccountMeta
	Data     []byte
}

// NewInstruction creates a new instruction.
func NewInstruction(program ed25519.PublicKey, data []byte, accounts ...AccountMeta) Instruction {
	return Instruction{
		Program:  program,
		Data:     data,
		Accounts: accounts,
	}
}

// CompiledInstruction represents an instruction that has been compiled into a transaction.
type CompiledInstruction struct {
	ProgramIndex byte
	Accounts     []byte
	Data         []byte
}
