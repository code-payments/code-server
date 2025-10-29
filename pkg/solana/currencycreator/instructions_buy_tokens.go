package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	BuyTokensInstructionArgsSize = (8 + // in_amount
		8) // min_amount_out
)

type BuyTokensInstructionArgs struct {
	InAmount     uint64
	MinAmountOut uint64
}

type BuyTokensInstructionAccounts struct {
	Buyer       ed25519.PublicKey
	Pool        ed25519.PublicKey
	Currency    ed25519.PublicKey
	TargetMint  ed25519.PublicKey
	BaseMint    ed25519.PublicKey
	VaultTarget ed25519.PublicKey
	VaultBase   ed25519.PublicKey
	BuyerTarget ed25519.PublicKey
	BuyerBase   ed25519.PublicKey
	FeeTarget   ed25519.PublicKey
	FeeBase     ed25519.PublicKey
}

func NewBuyTokensInstruction(
	accounts *BuyTokensInstructionAccounts,
	args *BuyTokensInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+BuyTokensInstructionArgsSize)

	putInstructionType(data, InstructionTypeBuyTokens, &offset)
	putUint64(data, args.InAmount, &offset)
	putUint64(data, args.MinAmountOut, &offset)

	return solana.Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []solana.AccountMeta{
			{
				PublicKey:  accounts.Buyer,
				IsWritable: true,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Pool,
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
				PublicKey:  accounts.VaultTarget,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VaultBase,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.BuyerTarget,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.BuyerBase,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.FeeTarget,
				IsWritable: true,
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
		},
	}
}
