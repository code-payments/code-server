package token

import (
	"bytes"
	"crypto/ed25519"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// AssociatedTokenAccountProgramKey  is the address of the associated token account program that should be used.
//
// Current key: ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL
var AssociatedTokenAccountProgramKey = ed25519.PublicKey{140, 151, 37, 143, 78, 36, 137, 241, 187, 61, 16, 41, 20, 142, 13, 131, 11, 90, 19, 153, 218, 255, 16, 132, 4, 142, 123, 216, 219, 233, 248, 89}

// GetAssociatedAccount returns the associated account address for an SPL token.
//
// Reference: https://spl.solana.com/associated-token-account#finding-the-associated-token-account-address
func GetAssociatedAccount(wallet, mint ed25519.PublicKey) (ed25519.PublicKey, error) {
	return solana.FindProgramAddress(
		AssociatedTokenAccountProgramKey,
		wallet,
		ProgramKey,
		mint,
	)
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/0639953c7dd0f5228c3ceda3ba68fece3b46ff1d/associated-token-account/program/src/lib.rs#L54
func CreateAssociatedTokenAccount(subsidizer, wallet, mint ed25519.PublicKey) (solana.Instruction, ed25519.PublicKey, error) {
	addr, err := GetAssociatedAccount(wallet, mint)
	if err != nil {
		return solana.Instruction{}, nil, err
	}

	return solana.NewInstruction(
		AssociatedTokenAccountProgramKey,
		[]byte{},
		solana.NewAccountMeta(subsidizer, true),
		solana.NewAccountMeta(addr, false),
		solana.NewReadonlyAccountMeta(wallet, false),
		solana.NewReadonlyAccountMeta(mint, false),
		solana.NewReadonlyAccountMeta(system.ProgramKey[:], false),
		solana.NewReadonlyAccountMeta(ProgramKey, false),
		solana.NewReadonlyAccountMeta(system.RentSysVar, false),
	), addr, nil
}

type DecompiledCreateAssociatedAccount struct {
	Subsidizer ed25519.PublicKey
	Address    ed25519.PublicKey
	Owner      ed25519.PublicKey
	Mint       ed25519.PublicKey
}

func DecompileCreateAssociatedAccount(m solana.Message, index int) (*DecompiledCreateAssociatedAccount, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]
	if !bytes.Equal(m.Accounts[i.ProgramIndex], AssociatedTokenAccountProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if len(i.Data) != 0 {
		return nil, errors.Errorf("unexpected data")
	}
	if len(i.Accounts) != 7 {
		return nil, errors.Errorf("invalid number of accounts: %d (expected %d)", len(i.Accounts), 7)
	}

	if !bytes.Equal(m.Accounts[i.Accounts[4]], system.ProgramKey[:]) {
		return nil, errors.Errorf("system program key mismatch")
	}
	if !bytes.Equal(m.Accounts[i.Accounts[5]], ProgramKey) {
		return nil, errors.Errorf("token program key mismatch")
	}
	if !bytes.Equal(m.Accounts[i.Accounts[6]], system.RentSysVar) {
		return nil, errors.Errorf("rent sysvar mismatch")
	}

	return &DecompiledCreateAssociatedAccount{
		Subsidizer: m.Accounts[i.Accounts[0]],
		Address:    m.Accounts[i.Accounts[1]],
		Owner:      m.Accounts[i.Accounts[2]],
		Mint:       m.Accounts[i.Accounts[3]],
	}, nil
}
