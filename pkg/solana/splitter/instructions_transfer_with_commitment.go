package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var transferWithCommitmentInstructionDiscriminator = []byte{
	177, 247, 125, 208, 153, 166, 205, 120,
}

const (
	TransferWithCommitmentInstructionArgsSize = (1 + // pool_bump
		8 + // amount
		32 + // transcript_hash
		32) // recent_root

	TransferWithCommitmentInstructionAccountsSize = (32 + // pool
		32 + // vault
		32 + // destination
		32 + // commitment
		32 + // authority
		32 + // payer
		32 + // splTokenProgram
		32) // systemProgram

	TransferWithCommitmentInstructionSize = (8 + // discriminator
		TransferWithCommitmentInstructionArgsSize + // args
		TransferWithCommitmentInstructionAccountsSize) // accounts
)

type TransferWithCommitmentInstructionArgs struct {
	PoolBump   uint8
	Amount     uint64
	Transcript Hash
	RecentRoot Hash
}

type TransferWithCommitmentInstructionAccounts struct {
	Pool        ed25519.PublicKey
	Vault       ed25519.PublicKey
	Destination ed25519.PublicKey
	Commitment  ed25519.PublicKey
	Authority   ed25519.PublicKey
	Payer       ed25519.PublicKey
}

func NewTransferWithCommitmentInstruction(
	accounts *TransferWithCommitmentInstructionAccounts,
	args *TransferWithCommitmentInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(transferWithCommitmentInstructionDiscriminator)+
			TransferWithCommitmentInstructionArgsSize)

	putDiscriminator(data, transferWithCommitmentInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)
	putUint64(data, args.Amount, &offset)
	putHash(data, args.Transcript, &offset)
	putHash(data, args.RecentRoot, &offset)

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Pool,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Vault,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Destination,
				IsWritable: true,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Commitment,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Authority,
				IsWritable: false,
				IsSigner:   true,
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
			{
				PublicKey:  SYSVAR_RENT_PUBKEY,
				IsWritable: false,
				IsSigner:   false,
			},
		},
	}
}

func TransferWithCommitmentInstructionFromBinary(data []byte) (*TransferWithCommitmentInstructionArgs, *TransferWithCommitmentInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < TransferWithCommitmentInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, transferWithCommitmentInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args TransferWithCommitmentInstructionArgs
	var accounts TransferWithCommitmentInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getUint64(data, &args.Amount, &offset)
	getHash(data, &args.Transcript, &offset)
	getHash(data, &args.RecentRoot, &offset)

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Vault, &offset)
	getKey(data, &accounts.Destination, &offset)
	getKey(data, &accounts.Commitment, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
