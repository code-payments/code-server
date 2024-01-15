package swap_validator

import (
	"crypto/ed25519"
	"errors"
)

var (
	ErrInvalidProgram         = errors.New("invalid program id")
	ErrInvalidAccountData     = errors.New("unexpected account data")
	ErrInvalidInstructionData = errors.New("unexpected instruction data")
)

var (
	PROGRAM_ADDRESS = mustBase58Decode("sWvA66HNNvgamibZe88v3NN5nQwE8tp3KitfViFjukA")
	PROGRAM_ID      = ed25519.PublicKey(PROGRAM_ADDRESS)
)

var (
	SYSTEM_PROGRAM_ID = ed25519.PublicKey(mustBase58Decode("11111111111111111111111111111111"))

	SYSVAR_RENT_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarRent111111111111111111111111111111111"))
)

// AccountMeta represents the account information required
// for building transactions.
type AccountMeta struct {
	PublicKey  ed25519.PublicKey
	IsWritable bool
	IsSigner   bool
}

// Instruction represents a transaction instruction.
type Instruction struct {
	Program  ed25519.PublicKey
	Accounts []AccountMeta
	Data     []byte
}
