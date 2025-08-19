package currencycreator

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
	PROGRAM_ADDRESS = mustBase58Decode("ccZLx5N31asHhCa7hFmvdC9EGYVam13L8WXPTjPEiJY")
	PROGRAM_ID      = ed25519.PublicKey(PROGRAM_ADDRESS)
)

var (
	METADATA_PROGRAM_ID  = ed25519.PublicKey(mustBase58Decode("metaqbxxUerdq28cj1RbAWkYQm3ybzjb6a8bt518x1s"))
	SPL_TOKEN_PROGRAM_ID = ed25519.PublicKey(mustBase58Decode("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"))
	SYSTEM_PROGRAM_ID    = ed25519.PublicKey(mustBase58Decode("11111111111111111111111111111111"))

	SYSVAR_RENT_PUBKEY = ed25519.PublicKey(mustBase58Decode("SysvarRent111111111111111111111111111111111"))
)
