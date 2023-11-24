package timelock_token

import (
	"bytes"
	"crypto/ed25519"
)

var burnDustWithAuthorityInstructionDiscriminator = []byte{
	39, 42, 255, 218, 14, 124, 78, 45,
}

const (
	BurnDustWithAuthorityInstructionArgsSize = (1 + // TimelockBump
		8) // max_amount

	BurnDustWithAuthorityInstructionAccountsSize = (32 + // timelock
		32 + // vault
		32 + // vaultOwner
		32 + // timeAuthority
		32 + // mint
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	BurnDustWithAuthorityInstructionSize = (8 + // discriminator
		BurnDustWithAuthorityInstructionArgsSize + // args
		BurnDustWithAuthorityInstructionAccountsSize) // accounts
)

type BurnDustWithAuthorityInstructionArgs struct {
	TimelockBump uint8
	MaxAmount    uint64
}

type BurnDustWithAuthorityInstructionAccounts struct {
	Timelock      ed25519.PublicKey
	Vault         ed25519.PublicKey
	VaultOwner    ed25519.PublicKey
	TimeAuthority ed25519.PublicKey
	Mint          ed25519.PublicKey
	Payer         ed25519.PublicKey
}

func NewBurnDustWithAuthorityInstruction(
	accounts *BurnDustWithAuthorityInstructionAccounts,
	args *BurnDustWithAuthorityInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(burnDustWithAuthorityInstructionDiscriminator)+
			BurnDustWithAuthorityInstructionArgsSize)

	putDiscriminator(data, burnDustWithAuthorityInstructionDiscriminator, &offset)
	putUint8(data, args.TimelockBump, &offset)
	putUint64(data, args.MaxAmount, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Timelock,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Vault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.VaultOwner,
				IsWritable: false,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.TimeAuthority,
				IsWritable: false,
				IsSigner:   true,
			},
			{
				PublicKey:  accounts.Mint,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Payer,
				IsWritable: true,
				IsSigner:   true,
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
		},
	}
}

func BurnDustWithAuthorityInstructionFromBinary(data []byte) (*BurnDustWithAuthorityInstructionArgs, *BurnDustWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < BurnDustWithAuthorityInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, burnDustWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args BurnDustWithAuthorityInstructionArgs
	var accounts BurnDustWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(data, &args.TimelockBump, &offset)
	getUint64(data, &args.MaxAmount, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Timelock, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.VaultOwner, &offset)
	getKey(data, &accounts.TimeAuthority, &offset)
	getKey(data, &accounts.Mint, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
