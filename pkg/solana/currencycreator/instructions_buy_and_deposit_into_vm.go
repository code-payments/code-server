package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

const (
	BuyAndDepositIntoVmInstructionArgsSize = (8 + // in_amount
		8 + // min_out_amount
		2) // account_index
)

type BuyAndDepositIntoVmInstructionArgs struct {
	InAmount      uint64
	MinOutAmount  uint64
	VmMemoryIndex uint16
}

type BuyAndDepositIntoVmInstructionAccounts struct {
	Buyer       ed25519.PublicKey
	Pool        ed25519.PublicKey
	Currency    ed25519.PublicKey
	TargetMint  ed25519.PublicKey
	BaseMint    ed25519.PublicKey
	VaultTarget ed25519.PublicKey
	VaultBase   ed25519.PublicKey
	BuyerBase   ed25519.PublicKey
	FeeTarget   ed25519.PublicKey
	FeeBase     ed25519.PublicKey

	VmAuthority ed25519.PublicKey
	Vm          ed25519.PublicKey
	VmMemory    ed25519.PublicKey
	VmOmnibus   ed25519.PublicKey
	VtaOwner    ed25519.PublicKey
}

func NewBuyAndDepositIntoVmInstruction(
	accounts *BuyAndDepositIntoVmInstructionAccounts,
	args *BuyAndDepositIntoVmInstructionArgs,
) solana.Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte, 1+BuyAndDepositIntoVmInstructionArgsSize)

	putInstructionType(data, InstructionTypeBuyAndDepositIntoVm, &offset)
	putUint64(data, args.InAmount, &offset)
	putUint64(data, args.MinOutAmount, &offset)
	putUint16(data, args.VmMemoryIndex, &offset)

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
				PublicKey:  accounts.VmMemory,
				IsWritable: true,
				IsSigner:   false,
			},

			{
				PublicKey:  accounts.VmOmnibus,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VtaOwner,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  SPL_TOKEN_PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  cvm.PROGRAM_ID,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}
