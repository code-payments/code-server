package cvm

import (
	"crypto/ed25519"
	"errors"
)

var (
	ErrInvalidProgram            = errors.New("invalid program id")
	ErrInvalidAccountData        = errors.New("unexpected account data")
	ErrInvalidVirtualAccountData = errors.New("unexpected virtual account data")
	ErrInvalidVirtualAccountType = errors.New("unexpected virtual account type")
	ErrInvalidInstructionData    = errors.New("unexpected instruction data")
)

var (
	// todo: setup real program address
	PROGRAM_ADDRESS = mustBase58Decode("vmT2hAx4N2U6DyjYxgQHER4VGC8tHJCfHNsSepBKCJZ")
	PROGRAM_ID      = ed25519.PublicKey(PROGRAM_ADDRESS)
)

var (
	SYSTEM_PROGRAM_ID    = ed25519.PublicKey(mustBase58Decode("11111111111111111111111111111111"))
	SPL_TOKEN_PROGRAM_ID = ed25519.PublicKey(mustBase58Decode("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"))
	TIMELOCK_PROGRAM_ID  = ed25519.PublicKey(mustBase58Decode("time2Z2SCnn3qYg3ULKVtdkh8YmZ5jFdKicnA1W2YnJ"))
	SPLITTER_PROGRAM_ID  = ed25519.PublicKey(mustBase58Decode("spLit2eb13Tz93if6aJM136nUWki5PVUsoEjcUjwpwW"))

	SYSVAR_RENT_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarRent111111111111111111111111111111111"))
)
