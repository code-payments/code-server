package splitter_token

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
	PROGRAM_ADDRESS = mustBase58Decode("spLit2eb13Tz93if6aJM136nUWki5PVUsoEjcUjwpwW")
	PROGRAM_ID      = ed25519.PublicKey(PROGRAM_ADDRESS)
)

var (
	SYSTEM_PROGRAM_ID               = ed25519.PublicKey(mustBase58Decode("11111111111111111111111111111111"))
	SPL_TOKEN_PROGRAM_ID            = ed25519.PublicKey(mustBase58Decode("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"))
	SPL_ASSOCIATED_TOKEN_PROGRAM_ID = ed25519.PublicKey(mustBase58Decode("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"))

	SYSVAR_CLOCK_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarC1ock11111111111111111111111111111111"))
	SYSVAR_RENT_PUBKEY  = ed25519.PublicKey(mustBase58Decode("SysvarRent111111111111111111111111111111111"))
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
