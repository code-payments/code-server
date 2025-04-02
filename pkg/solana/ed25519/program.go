package ed25519

import (
	"crypto/ed25519"
	"encoding/binary"
	"math"

	"github.com/code-payments/code-server/pkg/solana"
)

// Ed25519SigVerify111111111111111111111111111
var ProgramKey = ed25519.PublicKey{3, 125, 70, 214, 124, 147, 251, 190, 18, 249, 66, 143, 131, 141, 64, 255, 5, 112, 116, 73, 39, 244, 138, 100, 252, 202, 112, 68, 128, 0, 0, 0}

// Reference: https://github.com/solana-labs/solana/blob/27eff8408b7223bb3c4ab70523f8a8dca3ca6645/sdk/src/ed25519_instruction.rs#L32
func Instruction(privateKey ed25519.PrivateKey, message []byte) solana.Instruction {
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signature := ed25519.Sign(privateKey, message)

	data := make([]byte, 112+len(message))

	offset := 0

	data[offset] = 1 // num_signatures
	offset++

	data[offset] = 0 // padding
	offset++

	binary.LittleEndian.PutUint16(data[offset:], 48) // signature_offset
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], math.MaxUint16) // signature_instruction_index
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], 16) // public_key_offset
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], math.MaxUint16) // public_key_instruction_index
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], 112) // message_data_offset
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], uint16(len(message))) // message_data_size
	offset += 2

	binary.LittleEndian.PutUint16(data[offset:], math.MaxUint16) // message_instruction_index
	offset += 2

	copy(data[offset:], publicKey)
	offset += 32

	copy(data[offset:], signature)
	offset += 64

	copy(data[offset:], message)

	return solana.NewInstruction(
		ProgramKey,
		data,
	)
}
