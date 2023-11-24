package timelock_token

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	expectedInitOffset = 0
	pdaVersion1        = 1
)

var (
	statePrefix = []byte("timelock_state")
	vaultPrefix = []byte("timelock_vault")

	pdaPadding = make([]byte, ed25519.PublicKeySize)
)

type GetStateAddressArgs struct {
	Mint           ed25519.PublicKey
	TimeAuthority  ed25519.PublicKey
	Nonce          ed25519.PublicKey
	VaultOwner     ed25519.PublicKey
	UnlockDuration uint64
}

type GetVaultAddressArgs struct {
	State ed25519.PublicKey
}

func GetStateAddress(args *GetStateAddressArgs) (ed25519.PublicKey, uint8, error) {
	unlockDurationBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(unlockDurationBytes, args.UnlockDuration)

	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		statePrefix,
		[]byte{pdaVersion1},
		args.Mint,
		args.TimeAuthority,
		args.Nonce,
		args.VaultOwner,
		unlockDurationBytes,
		pdaPadding,
	)
}

func GetVaultAddress(args *GetVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		vaultPrefix,
		args.State,
		[]byte{expectedInitOffset},
	)
}
