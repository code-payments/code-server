package currencycreator

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
)

const (
	InitializePoolInstructionArgsSize = (8 + // supply
		RawExponentialCurveSize + // curve
		8 + // purchase_cap
		8 + // sale_cap
		4 + // buy_fee
		4 + // sell_fee
		8 + // go_live_wait_time
		1 + // bump
		1 + // vault_target_bump
		1 + // vault_base_bump
		5) // padding
)

type InitializePoolInstructionArgs struct {
	Supply          uint64
	Curve           RawExponentialCurve
	PurchaseCap     uint64
	SaleCap         uint64
	BuyFee          uint32
	SellFee         uint32
	GoLiveWaitTime  uint64
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
	putUint64(data, args.Supply, &offset)
	putRawExponentialCurve(data, args.Curve, &offset)
	putUint64(data, args.PurchaseCap, &offset)
	putUint64(data, args.SaleCap, &offset)
	putUint32(data, args.BuyFee, &offset)
	putUint32(data, args.SellFee, &offset)
	putUint64(data, args.GoLiveWaitTime, &offset)
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
