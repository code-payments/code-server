package splitter_token

import (
	"bytes"
	"crypto/ed25519"
)

var uploadProofInstructionDiscriminator = []byte{
	57, 235, 171, 213, 237, 91, 79, 2,
}

const (
	UploadProofInstructionArgsSize = (1 + // PoolBump
		1 + // ProofBump
		1 + // CurrentSize
		1 + // DataSize
		4) // Data

	UploadProofInstructionAccountsSize = (32 + // pool
		32 + // vault
		32 + // mint
		32 + // authority
		32 + // payer
		32 + // splTokenProgram
		32 + // systemProgram
		32) // sysvarRent

	UploadProofInstructionSize = (8 + // discriminator
		UploadProofInstructionArgsSize + // args
		UploadProofInstructionAccountsSize) // accounts
)

type UploadProofInstructionArgs struct {
	PoolBump    uint8
	ProofBump   uint8
	CurrentSize uint8
	DataSize    uint8
	Data        []Hash
}

type UploadProofInstructionAccounts struct {
	Pool      ed25519.PublicKey
	Proof     ed25519.PublicKey
	Authority ed25519.PublicKey
	Payer     ed25519.PublicKey
}

func NewUploadProofInstruction(
	accounts *UploadProofInstructionAccounts,
	args *UploadProofInstructionArgs,
) Instruction {
	var offset int

	// Serialize instruction arguments
	data := make([]byte,
		len(uploadProofInstructionDiscriminator)+
			UploadProofInstructionArgsSize+
			32*len(args.Data))

	putDiscriminator(data, uploadProofInstructionDiscriminator, &offset)
	putUint8(data, args.PoolBump, &offset)
	putUint8(data, args.ProofBump, &offset)
	putUint8(data, args.CurrentSize, &offset)
	putUint8(data, uint8(len(args.Data)), &offset)

	putUint32(data, uint32(len(args.Data)), &offset)
	for _, item := range args.Data {
		putHash(data, item[:], &offset)
	}

	return Instruction{
		Program: PROGRAM_ADDRESS,

		// Instruction args
		Data: data,

		// Instruction accounts
		Accounts: []AccountMeta{
			{
				PublicKey:  accounts.Pool,
				IsWritable: false,
				IsSigner:   false,
			},
			{
				PublicKey:  accounts.Proof,
				IsWritable: true,
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
		},
	}
}

func UploadProofInstructionFromBinary(data []byte) (*UploadProofInstructionArgs, *UploadProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	if len(data) < UploadProofInstructionSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(data, &discriminator, &offset)

	if !bytes.Equal(discriminator, uploadProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args UploadProofInstructionArgs
	var accounts UploadProofInstructionAccounts

	// Instruction Args
	getUint8(data, &args.PoolBump, &offset)
	getUint8(data, &args.ProofBump, &offset)
	getUint8(data, &args.CurrentSize, &offset)
	getUint8(data, &args.DataSize, &offset)

	var dataLength uint32
	getUint32(data, &dataLength, &offset)

	args.Data = make([]Hash, args.DataSize)
	for i := uint32(0); i < dataLength; i++ {
		getHash(data, &args.Data[i], &offset)
	}

	// Instruction Accounts
	getKey(data, &accounts.Pool, &offset)
	getKey(data, &accounts.Proof, &offset)
	getKey(data, &accounts.Authority, &offset)
	getKey(data, &accounts.Payer, &offset)

	return &args, &accounts, nil
}
