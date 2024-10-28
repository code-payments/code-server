package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitRelayInstructionArgsSize = (MaxRelayAccountNameSize + // name
		1 + // relay_bump
		1) // relay_vault_bump
)

type InitRelayInstructionArgs struct {
	Name           string
	RelayBump      uint8
	RelayVaultBump uint8
}

type InitRelayInstructionAccounts struct {
	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	Relay       ed25519.PublicKey
	RelayVault  ed25519.PublicKey
	Mint        ed25519.PublicKey
}

func NewInitRelayInstruction(
	accounts *InitRelayInstructionAccounts,
	args *InitRelayInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitRelayInstructionArgsSize)

	putCodeInstruction(data, CodeInstructionInitRelay, &offset)
	putFixedString(data, args.Name, MaxRelayAccountNameSize, &offset)
	putUint8(data, args.RelayBump, &offset)
	putUint8(data, args.RelayVaultBump, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
			{
				PublicKey:  accounts.VmAuthority,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Vm,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Relay,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.RelayVault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSTEM_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SYSVAR_RENT_PUBKEY,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
