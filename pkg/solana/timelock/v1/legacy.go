// Temporary conversion to legacy way of doing things in the solana package.
//
// todo: Support remaining instructions
// todo: Migrate solana package to new ways of doing things

package timelock_token

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

func InitializeInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*InitializeInstructionArgs, *InitializeInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(initializeInstructionDiscriminator)+InitializeInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, initializeInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args InitializeInstructionArgs
	var accounts InitializeInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.NumDaysLocked, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.VaultOwner = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Mint = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.TimeAuthority = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}

func TransferWithAuthorityInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*TransferWithAuthorityInstructionArgs, *TransferWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(transferWithAuthorityInstructionDiscriminator)+TransferWithAuthorityInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, transferWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args TransferWithAuthorityInstructionArgs
	var accounts TransferWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)
	getUint64(instruction.Data, &args.Amount, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.VaultOwner = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.TimeAuthority = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Destination = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}

func WithdrawInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*WithdrawInstructionArgs, *WithdrawInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(withdrawInstructionDiscriminator)+WithdrawInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, withdrawInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args WithdrawInstructionArgs
	var accounts WithdrawInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.VaultOwner = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Destination = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[4]]

	return &args, &accounts, nil
}

func BurnDustWithAuthorityInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*BurnDustWithAuthorityInstructionArgs, *BurnDustWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(burnDustWithAuthorityInstructionDiscriminator)+BurnDustWithAuthorityInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, burnDustWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args BurnDustWithAuthorityInstructionArgs
	var accounts BurnDustWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)
	getUint64(instruction.Data, &args.MaxAmount, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.VaultOwner = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.TimeAuthority = txn.Message.Accounts[instruction.Accounts[3]]
	accounts.Mint = txn.Message.Accounts[instruction.Accounts[4]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[5]]

	return &args, &accounts, nil
}

func RevokeLockWithAuthorityFromLegacyInstruction(txn solana.Transaction, idx int) (*RevokeLockWithAuthorityInstructionArgs, *RevokeLockWithAuthorityInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(revokeLockWithAuthorityInstructionDiscriminator)+RevokeLockWithAuthorityInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, revokeLockWithAuthorityInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args RevokeLockWithAuthorityInstructionArgs
	var accounts RevokeLockWithAuthorityInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.TimeAuthority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}

func DeactivateInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*DeactivateInstructionArgs, *DeactivateInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(deactivateInstructionDiscriminator)+DeactivateInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, deactivateInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args DeactivateInstructionArgs
	var accounts DeactivateInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.VaultOwner = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[2]]

	return &args, &accounts, nil
}

func CloseAccountsInstructionFromLegacyInstruction(txn solana.Transaction, idx int) (*CloseAccountsInstructionArgs, *CloseAccountsInstructionAccounts, error) {
	var offset int
	var discriminator []byte

	instruction := txn.Message.Instructions[idx]

	programAccount := txn.Message.Accounts[instruction.ProgramIndex]
	if !bytes.Equal(PROGRAM_ADDRESS, programAccount) {
		return nil, nil, ErrInvalidInstructionData
	}

	if len(instruction.Data) < len(closeAccountsInstructionDiscriminator)+CloseAccountsInstructionArgsSize {
		return nil, nil, ErrInvalidInstructionData
	}

	getDiscriminator(instruction.Data, &discriminator, &offset)

	if !bytes.Equal(discriminator, closeAccountsInstructionDiscriminator) {
		return nil, nil, ErrInvalidInstructionData
	}

	var args CloseAccountsInstructionArgs
	var accounts CloseAccountsInstructionAccounts

	// Instruction Args
	getUint8(instruction.Data, &args.TimelockBump, &offset)

	// Instruction Accounts
	accounts.Timelock = txn.Message.Accounts[instruction.Accounts[0]]
	accounts.Vault = txn.Message.Accounts[instruction.Accounts[1]]
	accounts.CloseAuthority = txn.Message.Accounts[instruction.Accounts[2]]
	accounts.Payer = txn.Message.Accounts[instruction.Accounts[3]]

	return &args, &accounts, nil
}
