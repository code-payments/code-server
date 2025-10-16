package address_lookup_table

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/binary"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// Reference: https://github.com/solana-program/address-lookup-table/blob/main/program/src/instruction.rs

// AddressLookupTab1e1111111111111111111111111
var ProgramKey = ed25519.PublicKey{2, 119, 166, 175, 151, 51, 155, 122, 200, 141, 24, 146, 201, 4, 70, 245, 0, 2, 48, 146, 102, 246, 46, 83, 193, 24, 36, 73, 130, 0, 0, 0}

const (
	commandCreateLookupTable uint32 = iota
	commandFreezeLookupTable
	commandExtendLookupTable
	commandDeactivateLookupTable
	commandCloseLookupTable
)

func Create(alt, authority, payer ed25519.PublicKey, recentSlot uint64, bumpSeed uint8) solana.Instruction {
	data := make([]byte, 4+8+1)

	var offset int
	binary.PutUint32(data[offset:], commandCreateLookupTable, &offset)
	binary.PutUint64(data[offset:], recentSlot, &offset)
	binary.PutUint8(data[offset:], bumpSeed, &offset)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(alt, false),
		solana.NewReadonlyAccountMeta(authority, true),
		solana.NewAccountMeta(payer, true),
		solana.NewReadonlyAccountMeta(system.ProgramKey[:], false),
	)
}

func Extend(alt, authority, payer ed25519.PublicKey, addresses ...ed25519.PublicKey) solana.Instruction {
	data := make([]byte, 4+8+len(addresses)*ed25519.PublicKeySize)

	var offset int
	binary.PutUint32(data[offset:], commandExtendLookupTable, &offset)
	binary.PutUint64(data[offset:], uint64(len(addresses)), &offset)
	for _, address := range addresses {
		binary.PutKey32(data[offset:], address, &offset)
	}

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(alt, false),
		solana.NewReadonlyAccountMeta(authority, true),
		solana.NewAccountMeta(payer, true),
		solana.NewReadonlyAccountMeta(system.ProgramKey[:], false),
	)
}
