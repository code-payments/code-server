package timelock_token

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
	PROGRAM_ADDRESS = mustBase58Decode("time2Z2SCnn3qYg3ULKVtdkh8YmZ5jFdKicnA1W2YnJ")
	PROGRAM_ID      = ed25519.PublicKey(PROGRAM_ADDRESS)
)

var (
	SYSTEM_PROGRAM_ID               = ed25519.PublicKey(mustBase58Decode("11111111111111111111111111111111"))
	SPL_TOKEN_PROGRAM_ID            = ed25519.PublicKey(mustBase58Decode("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"))
	SPL_ASSOCIATED_TOKEN_PROGRAM_ID = ed25519.PublicKey(mustBase58Decode("ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"))

	SYSVAR_CLOCK_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarC1ock11111111111111111111111111111111"))
	SYSVAR_RENT_PUBKEY  = ed25519.PublicKey(mustBase58Decode("SysvarRent111111111111111111111111111111111"))

	/*
		// Unused
		SYSVAR_EPOCH_SCHEDULE_PUBKEY     = ed25519.PublicKey(mustBase58Decode("SysvarEpochSchedu1e111111111111111111111111"))
		SYSVAR_INSTRUCTIONS_PUBKEY       = ed25519.PublicKey(mustBase58Decode("Sysvar1nstructions1111111111111111111111111"))
		SYSVAR_RECENT_BLOCKHASHES_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarRecentB1ockHashes11111111111111111111"))
		SYSVAR_REWARDS_PUBKEY            = ed25519.PublicKey(mustBase58Decode("SysvarRewards111111111111111111111111111111"))
		SYSVAR_SLOT_HASHES_PUBKEY        = ed25519.PublicKey(mustBase58Decode("SysvarS1otHashes111111111111111111111111111"))
		SYSVAR_SLOT_HISTORY_PUBKEY       = ed25519.PublicKey(mustBase58Decode("SysvarS1otHistory11111111111111111111111111"))
		SYSVAR_STAKE_HISTORY_PUBKEY      = ed25519.PublicKey(mustBase58Decode("SysvarStakeHistory1111111111111111111111111"))
	*/
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
