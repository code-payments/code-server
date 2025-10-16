package address_lookup_table

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/code-payments/code-server/pkg/solana"
)

func GetAddress(authority ed25519.PublicKey, recentSlot uint64) (ed25519.PublicKey, uint8, error) {
	var recentSlotBytes [8]byte
	binary.LittleEndian.PutUint64(recentSlotBytes[:], recentSlot)

	return solana.FindProgramAddressAndBump(
		ProgramKey,
		authority,
		recentSlotBytes[:],
	)
}
