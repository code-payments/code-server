package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitializeMetadataInstructionArgsSize = 0
)

type InitializeMetadataInstructionArgs struct {
}

type InitializeMetadataInstructionAccounts struct {
	Authority ed25519.PublicKey
	Mint      ed25519.PublicKey
	Currency  ed25519.PublicKey
	Metadata  ed25519.PublicKey
}

func NewInitializeMetadataInstruction(
	accounts *InitializeMetadataInstructionAccounts,
	args *InitializeMetadataInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitializeMetadataInstructionArgsSize)

	putInstructionType(data, InstructionTypeInitializeMetadata, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
			{
				PublicKey:  accounts.Authority,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Currency,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Metadata,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  METADATA_PROGRAM_ID,
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
