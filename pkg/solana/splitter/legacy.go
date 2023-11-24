// Temporary conversion to legacy way of doing things in the solana package.
//
// todo: Support all instructions that were recently brought in
// todo: Migrate solana package to new ways of doing things

package splitter_token

import (
	"bytes"

	"github.com/code-payments/code-server/pkg/solana"
)

func (i Instruction) ToLegacyInstruction() solana.Instruction {
	legacyAccountMeta := make([]solana.AccountMeta, len(i.Accounts))
	for i, accountMeta := range i.Accounts {
		legacyAccountMeta[i] = solana.AccountMeta{
			PublicKey:  accountMeta.PublicKey,
			IsSigner:   accountMeta.IsSigner,
			IsWritable: accountMeta.IsWritable,
		}
	}

	return solana.Instruction{
		Program:  PROGRAM_ID,
		Accounts: legacyAccountMeta,
		Data:     i.Data,
	}
}

func TransferWithCommitmentInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*TransferWithCommitmentInstructionArgs, *TransferWithCommitmentInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(transferWithCommitmentInstructionDiscriminator)+TransferWithCommitmentInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, transferWithCommitmentInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args TransferWithCommitmentInstructionArgs
	var accounts TransferWithCommitmentInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint64(instruction.Data, &args.Amount, &offset)
	getHash(instruction.Data, &args.Transcript, &offset)
	getHash(instruction.Data, &args.RecentRoot, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Destination = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Commitment = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}

func SaveRecentRootInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*SaveRecentRootInstructionArgs, *SaveRecentRootInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(saveRecentRootInstructionDiscriminator)+SaveRecentRootInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, saveRecentRootInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args SaveRecentRootInstructionArgs
	var accounts SaveRecentRootInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[2]]

	return &args, &accounts, nil
}

func InitializeProofInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*InitializeProofInstructionArgs, *InitializeProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(initializeProofInstructionDiscriminator)+InitializeProofInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, initializeProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args InitializeProofInstructionArgs
	var accounts InitializeProofInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getHash(instruction.Data, &args.MerkleRoot, &offset)
	getKey(instruction.Data, &args.Commitment, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}

func UploadProofInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*UploadProofInstructionArgs, *UploadProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(uploadProofInstructionDiscriminator)+UploadProofInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, uploadProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args UploadProofInstructionArgs
	var accounts UploadProofInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint8(instruction.Data, &args.ProofBump, &offset)
	getUint8(instruction.Data, &args.CurrentSize, &offset)
	getUint8(instruction.Data, &args.DataSize, &offset)

	var actualSize uint32
	getUint32(instruction.Data, &actualSize, &offset)
	if actualSize != uint32(args.DataSize) {
		return nil, nil, ErrInvalidInstructionData
	}

	args.Data = make([]Hash, args.DataSize)
	for i := 0; i < int(args.DataSize); i++ {
		getHash(instruction.Data, &args.Data[i], &offset)
	}

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}

func VerifyProofInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*VerifyProofInstructionArgs, *VerifyProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(verifyProofInstructionDiscriminator)+VerifyProofInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, verifyProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args VerifyProofInstructionArgs
	var accounts VerifyProofInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint8(instruction.Data, &args.ProofBump, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}

func OpenTokenAccountInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*OpenTokenAccountInstructionArgs, *OpenTokenAccountInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(openTokenAccountInstructionDiscriminator)+OpenTokenAccountInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, openTokenAccountInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args OpenTokenAccountInstructionArgs
	var accounts OpenTokenAccountInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint8(instruction.Data, &args.ProofBump, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.CommitmentVault = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Mint = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}

func CloseProofInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*CloseProofInstructionArgs, *CloseProofInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(closeProofInstructionDiscriminator)+CloseProofInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeProofInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseProofInstructionArgs
	var accounts CloseProofInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint8(instruction.Data, &args.ProofBump, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}

func CloseTokenAccountInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*CloseTokenAccountInstructionArgs, *CloseTokenAccountInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(closeTokenAccountInstructionDiscriminator)+CloseTokenAccountInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeTokenAccountInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseTokenAccountInstructionArgs
	var accounts CloseTokenAccountInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.PoolBump, &offset)
	getUint8(instruction.Data, &args.ProofBump, &offset)
	getUint8(instruction.Data, &args.VaultBump, &offset)

	// Instruction Accounts
	accounts.Pool = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Proof = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.CommitmentVault = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.PoolVault = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Authority = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}
