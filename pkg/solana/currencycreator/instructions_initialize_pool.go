package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitializePoolInstructionArgsSize = (2 + // buy_fee
		2 + // sell_fee
		1 + // bump
		1 + // vault_target_bump
		1 + // vault_base_bump
		1) // padding
)

type InitializePoolInstructionArgs struct {
	BuyFee          uint16
	SellFee         uint16
	Bump            uint8
	VaultTargetBump uint8
	VaultBaseBump   uint8
}

type InitializePoolInstructionAccounts struct {
	Authority   ed25519.PublicKey
	Currency    ed25519.PublicKey
	TargetMint  ed25519.PublicKey
	BaseMint    ed25519.PublicKey
	Pool        ed25519.PublicKey
	VaultTarget ed25519.PublicKey
	VaultBase   ed25519.PublicKey
	FeeTarget   ed25519.PublicKey
	FeeBase     ed25519.PublicKey
}

func NewInitializePoolInstruction(
	accounts *InitializePoolInstructionAccounts,
	args *InitializePoolInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+InitializePoolInstructionArgsSize)

	putInstructionType(data, InstructionTypeInitializePool, &offset)
	putUint16(data, args.BuyFee, &offset)
	putUint16(data, args.SellFee, &offset)
	putUint8(data, args.Bump, &offset)
	putUint8(data, args.VaultTargetBump, &offset)
	putUint8(data, args.VaultBaseBump, &offset)

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
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.TargetMint,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.BaseMint,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Pool,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VaultTarget,
				IsWritable: true,
				IsSigner:   false,
			}, {
				PublicKey:  accounts.VaultBase,
				IsWritable: true,
				IsSigner:   false,
			}, {
				PublicKey:  accounts.FeeTarget,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.FeeBase,
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
