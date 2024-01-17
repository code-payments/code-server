package compute_budget

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"

	"github.com/code-payments/code-server/pkg/solana"
)

// ComputeBudget111111111111111111111111111111
var ProgramKey = ed25519.PublicKey{3, 6, 70, 111, 229, 33, 23, 50, 255, 236, 173, 186, 114, 195, 155, 231, 188, 140, 229, 187, 197, 247, 18, 107, 44, 67, 155, 58, 64, 0, 0, 0}

const (
	commandRequestUnits uint8 = iota
	commandRequestHeapFrame
	commandSetComputeUnitLimit
	commandSetComputeUnitPrice
)

func SetComputeUnitLimit(computeUnitLimit uint32) solana.Instruction {
	data := make([]byte, 1+4)
	data[0] = commandSetComputeUnitLimit
	binary.LittleEndian.PutUint32(data[1:], computeUnitLimit)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
	)
}

func SetComputeUnitPrice(computeUnitPrice uint64) solana.Instruction {
	data := make([]byte, 1+8)
	data[0] = commandSetComputeUnitPrice
	binary.LittleEndian.PutUint64(data[1:], computeUnitPrice)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
	)
}

func ParseSetComputeUnitLimitIxnData(data []byte) (uint32, error) {
	if len(data) != 5 {
		return 0, errors.New("invalid length")
	}

	if data[0] != commandSetComputeUnitLimit {
		return 0, errors.New("invalid instruction")
	}

	return binary.LittleEndian.Uint32(data[1:]), nil
}

func ParseSetComputeUnitPriceIxnData(data []byte) (uint64, error) {
	if len(data) != 9 {
		return 0, errors.New("invalid length")
	}

	if data[0] != commandSetComputeUnitPrice {
		return 0, errors.New("invalid instruction")
	}

	return binary.LittleEndian.Uint64(data[1:]), nil
}
