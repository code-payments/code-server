package system

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/pkg/errors"
)

// ProgramKey is the public key of the System Program responsible for processing instructions.
var ProgramKey [32]byte

// Command constants representing the set of supported instructions.
const (
	CommandCreateAccount uint32 = iota
	CommandAssign
	CommandTransfer
	CommandCreateAccountWithSeed
	CommandAdvanceNonceAccount
	CommandWithdrawNonceAccount
	CommandInitializeNonceAccount
	CommandAuthorizeNonceAccount
	CommandAllocate
	CommandAllocateWithSeed
	CommandAssignWithSeed
	CommandTransferWithSeed
)

// CreateAccount constructs a Solana instruction to create a new account.
// It transfers lamports to the new account and allocates space for it, setting the owner to the specified program.
func CreateAccount(funder, address, owner ed25519.PublicKey, lamports, size uint64) solana.Instruction {
	data := make([]byte, 4+2*8+32)
	binary.LittleEndian.PutUint32(data, CommandCreateAccount)
	binary.LittleEndian.PutUint64(data[4:], lamports)
	binary.LittleEndian.PutUint64(data[4+8:], size)
	copy(data[4+2*8:], owner)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(funder, true),
		solana.NewAccountMeta(address, true),
	)
}

// DecompiledCreateAccount holds the decomposed data from a CreateAccount instruction for easier inspection and processing.
type DecompiledCreateAccount struct {
	Funder   ed25519.PublicKey
	Address  ed25519.PublicKey
	Lamports uint64
	Size     uint64
	Owner    ed25519.PublicKey
}

// DecompileCreateAccount extracts and parses the components of a CreateAccount instruction from a Solana message.
func DecompileCreateAccount(m solana.Message, index int) (*DecompiledCreateAccount, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Wrapf(solana.ErrInvalidInstructionIndex, "instruction index out of bounds: %d", index)
	}

	instruction := m.Instructions[index]
	if !bytes.Equal(m.Accounts[instruction.ProgramIndex], ProgramKey[:]) {
		return nil, solana.ErrIncorrectProgram
	}

	if len(instruction.Data) < 52 { // Ensure there's enough data to parse
		return nil, errors.Errorf("expected data size of at least 52 bytes, got %d", len(instruction.Data))
	}

	command := binary.LittleEndian.Uint32(instruction.Data)
	if command != CommandCreateAccount {
		return nil, solana.ErrIncorrectInstruction
	}

	return &DecompiledCreateAccount{
		Funder:   m.Accounts[instruction.Accounts[0]],
		Address:  m.Accounts[instruction.Accounts[1]],
		Lamports: binary.LittleEndian.Uint64(instruction.Data[4:]),
		Size:     binary.LittleEndian.Uint64(instruction.Data[12:]),
		Owner:    ed25519.PublicKey(instruction.Data[20:52]),
	}, nil
}

// AdvanceNonce constructs an instruction to advance the nonce in a nonce account.
func AdvanceNonce(nonce, authority ed25519.PublicKey) solana.Instruction {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, CommandAdvanceNonceAccount)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(authority, true),
	)
}

// DecompiledAdvanceNonce holds the decomposed data from an AdvanceNonce instruction.
type DecompiledAdvanceNonce struct {
	Nonce     ed25519.PublicKey
	Authority ed25519.PublicKey
}

// DecompileAdvanceNonce extracts and parses the components of an AdvanceNonce instruction from a Solana message.
func DecompileAdvanceNonce(m solana.Message, index int) (*DecompiledAdvanceNonce, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Wrapf(solana.ErrInvalidInstructionIndex, "instruction index out of bounds: %d", index)
	}

	instruction := m.Instructions[index]
	if !bytes.Equal(m.Accounts[instruction.ProgramIndex], ProgramKey[:]) {
		return nil, solana.ErrIncorrectProgram
	}

	// Validate that the instruction data length matches what is expected for AdvanceNonceAccount.
	// Since AdvanceNonce only includes the command identifier, it should be exactly 4 bytes long.
	if len(instruction.Data) != 4 {
		return nil, errors.Errorf("expected data size of 4 bytes for AdvanceNonce, got %d", len(instruction.Data))
	}

	// Verify that the command encoded in the instruction data matches CommandAdvanceNonceAccount.
	command := binary.LittleEndian.Uint32(instruction.Data)
	if command != CommandAdvanceNonceAccount {
		return nil, solana.ErrIncorrectInstruction
	}

	// Validate the expected number of accounts involved in AdvanceNonce operation.
	// Typically, this includes the nonce account, the RecentBlockhashesSysVar, and the authority account.
	if len(instruction.Accounts) != 3 {
		return nil, errors.Errorf("expected 3 accounts for AdvanceNonce, got %d", len(instruction.Accounts))
	}

	// Extract and return the nonce and authority public keys from the message accounts.
	// The first account is the nonce account, and the third account is the authority (due to the ordering in AdvanceNonce instruction creation).
	return &DecompiledAdvanceNonce{
		Nonce:     m.Accounts[instruction.Accounts[0]],
		Authority: m.Accounts[instruction.Accounts[2]], // Assuming the third account is the authority based on typical instruction composition.
	}, nil
}

// GetNonceValueFromAccount extracts the nonce value from a given nonce account's data.
// This is crucial for operations requiring the current state of a nonce account, such as transaction signing.
func GetNonceValueFromAccount(info solana.AccountInfo) (solana.Blockhash, error) {
	// Instead of assuming a fixed size, check for a minimum size to ensure the necessary data is present.
	const minNonceAccountDataSize = 80 // Minimum expected size of a nonce account's data.
	const nonceValueOffset = 40        // Offset where the nonce value starts in the account data.

	var val solana.Blockhash
	if len(info.Data) < minNonceAccountDataSize {
		return val, errors.Errorf("invalid nonce account size: expected at least %d, got %d", minNonceAccountDataSize, len(info.Data))
	}
	if !bytes.Equal(info.Owner, ProgramKey[:]) {
		return val, errors.Errorf("invalid owner for nonce account: expected System Program")
	}

	copy(val[:], info.Data[nonceValueOffset:nonceValueOffset+32])
	return val, nil
}

// WithdrawNonce constructs an instruction to withdraw lamports from a nonce account.
func WithdrawNonce(nonce, auth, recipient ed25519.PublicKey, lamports uint64) solana.Instruction {
	data := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(data, CommandWithdrawNonceAccount)
	binary.LittleEndian.PutUint64(data[4:], lamports)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
		solana.NewAccountMeta(recipient, true),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(RentSysVar, false),
		solana.NewReadonlyAccountMeta(auth, true),
	)
}

// DecompiledWithdrawNonce holds the decomposed data from a WithdrawNonce instruction.
type DecompiledWithdrawNonce struct {
	Nonce     ed25519.PublicKey
	Auth      ed25519.PublicKey
	Recipient ed25519.PublicKey
	Amount    uint64
}

// DecompileWithdrawNonce extracts and parses the components of a WithdrawNonce instruction from a Solana message.
func DecompileWithdrawNonce(m solana.Message, index int) (*DecompiledWithdrawNonce, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Wrapf(solana.ErrInvalidInstructionIndex, "instruction index out of bounds: %d", index)
	}

	instruction := m.Instructions[index]
	if !bytes.Equal(m.Accounts[instruction.ProgramIndex], ProgramKey[:]) {
		return nil, solana.ErrIncorrectProgram
	}

	if len(instruction.Data) != 12 { // Ensure there's enough data to parse for WithdrawNonce
		return nil, errors.Errorf("expected data size of 12 bytes, got %d", len(instruction.Data))
	}

	command := binary.LittleEndian.Uint32(instruction.Data)
	if command != CommandWithdrawNonceAccount {
		return nil, solana.ErrIncorrectInstruction
	}

	return &DecompiledWithdrawNonce{
		Nonce:     m.Accounts[instruction.Accounts[0]],
		Recipient: m.Accounts[instruction.Accounts[1]],
		Auth:      m.Accounts[instruction.Accounts[2]],
		Amount:    binary.LittleEndian.Uint64(instruction.Data[4:]),
	}, nil
}

// InitializeNonce constructs an instruction to initialize a nonce account.
func InitializeNonce(nonce, auth ed25519.PublicKey) solana.Instruction {
	data := make([]byte, 4+32)
	binary.LittleEndian.PutUint32(data, CommandInitializeNonceAccount)
	copy(data[4:], auth[:])

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(RentSysVar, false),
	)
}

// AuthorizeNonce constructs an instruction to change the authorized entity for nonce instructions on an account.
func AuthorizeNonce(nonce, newAuth ed25519.PublicKey) solana.Instruction {
	data := make([]byte, 4+32)
	binary.LittleEndian.PutUint32(data, CommandAuthorizeNonceAccount)
	copy(data[4:], newAuth[:])

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
	)
}
