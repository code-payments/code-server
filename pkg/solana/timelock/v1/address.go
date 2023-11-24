package timelock_token

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var (
	statePrefix = []byte("timelock_state")
	vaultPrefix = []byte("timelock_vault")
)

type GetStateAddressArgs struct {
	Mint          ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	VaultOwner    ed25519.PublicKey
	NumDaysLocked uint8
}

type GetVaultAddressArgs struct {
	State       ed25519.PublicKey
	DataVersion TimelockDataVersion
}

func GetStateAddress(args *GetStateAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		statePrefix,
		args.Mint,
		args.TimeAuthority,
		args.VaultOwner,
		[]byte{args.NumDaysLocked},
	)
}

func GetVaultAddress(args *GetVaultAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		vaultPrefix,
		args.State,
		[]byte{byte(args.DataVersion)},
	)
}
