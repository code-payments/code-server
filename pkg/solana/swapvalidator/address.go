package swap_validator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

var (
	preSwapStatePrefix = []byte("pre_swap_state")
)

type GetPreSwapStateAddressArgs struct {
	Source      ed25519.PublicKey
	Destination ed25519.PublicKey
	Nonce       ed25519.PublicKey
}

func GetPreSwapStateAddress(args *GetPreSwapStateAddressArgs) (ed25519.PublicKey, uint8, error) {
	return solana.FindProgramAddressAndBump(
		PROGRAM_ID,
		preSwapStatePrefix,
		args.Source,
		args.Destination,
		args.Nonce,
	)
}
