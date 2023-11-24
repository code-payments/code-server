package memo

import (
	"bytes"
	"crypto/ed25519"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana"
)

// ProgramKey is the address of the memo program that should be used.
//
// Current key: Memo1UhkJRfHyvLMcVucJwxXeuD728EqVDDwQDxFMNo
//
// todo: lock this in, or make configurable
var ProgramKey = ed25519.PublicKey{5, 74, 83, 80, 248, 93, 200, 130, 214, 20, 165, 86, 114, 120, 138, 41, 109, 223, 30, 171, 171, 208, 166, 6, 120, 136, 73, 50, 244, 238, 246, 160}

// Reference: https://github.com/solana-labs/solana-program-library/blob/master/memo/program/src/entrypoint.rs
func Instruction(data string) solana.Instruction {
	return solana.NewInstruction(
		ProgramKey,
		[]byte(data),
	)
}

type DecompiledMemo struct {
	Data []byte
}

func DecompileMemo(m solana.Message, index int) (*DecompiledMemo, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}

	return &DecompiledMemo{Data: i.Data}, nil
}
