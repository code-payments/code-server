package system

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana"
)

var ProgramKey [32]byte

const (
	commandCreateAccount uint32 = iota
	// nolint:varcheck,deadcode,unused
	commandAssign
	// nolint:varcheck,deadcode,unused
	commandTransfer
	// nolint:varcheck,deadcode,unused
	commandCreateAccountWithSeed
	// nolint:varcheck,deadcode,unused
	commandAdvanceNonceAccount
	// nolint:varcheck,deadcode,unused
	commandWithdrawNonceAccount
	// nolint:varcheck,deadcode,unused
	commandInitializeNonceAccount
	// nolint:varcheck,deadcode,unused
	commandAuthorizeNonceAccount
	// nolint:varcheck,deadcode,unused
	commandAllocate
	// nolint:varcheck,deadcode,unused
	commandAllocateWithSeed
	// nolint:varcheck,deadcode,unused
	commandAssignWithSeed
	// nolint:varcheck,deadcode,unused
	commandTransferWithSeed
)

// Reference: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/system_instruction.rs#L58-L72
func CreateAccount(funder, address, owner ed25519.PublicKey, lamports, size uint64) solana.Instruction {
	// # Account references
	//   0. [WRITE, SIGNER] Funding account
	//   1. [WRITE, SIGNER] New account
	//
	// CreateAccount {
	//   // Number of lamports to transfer to the new account
	//   lamports: u64,
	//   // Number of bytes of memory to allocate
	//   space: u64,
	//
	//   //Address of program that will own the new account
	//   owner: Pubkey,
	// }
	//
	data := make([]byte, 4+2*8+32)
	binary.LittleEndian.PutUint32(data, commandCreateAccount)
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

type DecompiledCreateAccount struct {
	Funder  ed25519.PublicKey
	Address ed25519.PublicKey

	Lamports uint64
	Size     uint64
	Owner    ed25519.PublicKey
}

func DecompileCreateAccount(m solana.Message, index int) (*DecompiledCreateAccount, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], commandCreateAccount)
	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey[:]) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.HasPrefix(i.Data, prefix[:]) {
		return nil, solana.ErrIncorrectInstruction
	}

	if len(i.Accounts) != 2 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if len(i.Data) != 52 {
		return nil, errors.Errorf("invalid instruction data size: %d", len(i.Data))
	}

	v := &DecompiledCreateAccount{
		Funder:  m.Accounts[i.Accounts[0]],
		Address: m.Accounts[i.Accounts[1]],
	}
	v.Lamports = binary.LittleEndian.Uint64(i.Data[4:])
	v.Size = binary.LittleEndian.Uint64(i.Data[4+8:])
	v.Owner = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(v.Owner, i.Data[4+2*8:])

	return v, nil
}

// Reference: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/system_instruction.rs#L113-L119
func AdvanceNonce(nonce, authority ed25519.PublicKey) solana.Instruction {
	/// # Account references
	///   0. [WRITE, SIGNER] Nonce account
	///   1. [] RecentBlockhashes sysvar
	///   2. [SIGNER] Nonce authority
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, commandAdvanceNonceAccount)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, false),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(authority, true),
	)
}

type DecompiledAdvanceNonce struct {
	Nonce     ed25519.PublicKey
	Authority ed25519.PublicKey
}

func DecompileAdvanceNonce(m solana.Message, index int) (*DecompiledAdvanceNonce, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	var prefix [4]byte
	binary.LittleEndian.PutUint32(prefix[:], commandAdvanceNonceAccount)
	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey[:]) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.Equal(i.Data, prefix[:]) {
		return nil, solana.ErrIncorrectInstruction
	}
	if len(i.Accounts) != 3 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if !bytes.Equal(RecentBlockhashesSysVar, m.Accounts[i.Accounts[1]]) {
		return nil, errors.Errorf("invalid RecentBlockhashesSysVar")
	}

	return &DecompiledAdvanceNonce{
		Nonce:     m.Accounts[i.Accounts[0]],
		Authority: m.Accounts[i.Accounts[2]],
	}, nil
}

// GetNonceValueFromAccount returns the nonce value of a nonce account.
//
// Layout references:
// https://github.com/solana-labs/solana/blob/d7b9aca87b0327266cde4f0116113a4203642130/web3.js/src/nonce-account.js#L16-L22
// https://github.com/solana-labs/solana/blob/a4956844bdd081e7b90508066c579f29be306ce7/sdk/program/src/nonce/state/current.rs#L26
func GetNonceValueFromAccount(info solana.AccountInfo) (val solana.Blockhash, err error) {
	if len(info.Data) != 80 {
		return val, errors.Errorf("invalid nonce account size: %d", len(info.Data))
	}
	if !bytes.Equal(info.Owner, ProgramKey[:]) {
		return val, errors.Errorf("invalid nonce account (not owned by sys program)")
	}

	// (4)     u32: version
	// (4)     u32: size
	// (32) pubKey: authority
	// (32) pubkey: blockhash/value
	start := 4 + 4 + ed25519.PublicKeySize
	copy(val[:], info.Data[start:start+ed25519.PublicKeySize])
	return val, nil
}

// WithdrawNonce returns an instruction to withdraw funds from a nonce account
//
// The `uint64` parameter is the lamports to withdraw, which must leave the
// account balance above the rent exempt reserve or at zero.
//
// Reference: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/system_instruction.rs#L131
func WithdrawNonce(nonce, auth, receipient ed25519.PublicKey, lamports uint64) solana.Instruction {
	/// Withdraw funds from a nonce account
	///
	/// # Account references
	///   0. [WRITE] Nonce account
	///   1. [WRITE] Recipient account
	///   2. [] RecentBlockhashes sysvar
	///   3. [] Rent sysvar
	///   4. [SIGNER] Nonce authority
	///
	/// The `u64` parameter is the lamports to withdraw, which must leave the
	/// account balance above the rent exempt reserve or at zero.

	data := make([]byte, 4+8)
	binary.LittleEndian.PutUint32(data[:], commandWithdrawNonceAccount)
	binary.LittleEndian.PutUint64(data[4:], lamports)

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
		solana.NewAccountMeta(receipient, true),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(RentSysVar, false),
		solana.NewReadonlyAccountMeta(auth, true),
	)
}

type DecompiledWithdrawNonce struct {
	Nonce     ed25519.PublicKey
	Auth      ed25519.PublicKey
	Recipient ed25519.PublicKey
	Amount    uint64
}

func DecompileWithdrawNonce() {

}

// InitializeNonce returns an instruction to change the state of an Uninitalized nonce account to Initialized, setting the nonce value
//
// The `Pubkey` parameter specifies the entity authorized to execute nonce
// instruction on the account
//
// No signatures are required to execute this instruction, enabling derived
// nonce account addresses
//
// Reference: https://github.com/solana-labs/solana/blob/f02a78d8fff2dd7297dc6ce6eb5a68a3002f5359/sdk/src/system_instruction.rs#L146
func InitializeNonce(nonce, auth ed25519.PublicKey) solana.Instruction {
	/// # Account references
	///   0. [WRITE] Nonce account
	///   1. [] RecentBlockhashes sysvar
	///   2. [] Rent sysvar

	data := make([]byte, 4+32)
	binary.LittleEndian.PutUint32(data, commandInitializeNonceAccount)
	copy(data[4:], auth[:32])

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, false),
		solana.NewReadonlyAccountMeta(RecentBlockhashesSysVar, false),
		solana.NewReadonlyAccountMeta(RentSysVar, false),
	)
}

func DecompileInitializeNonce() {

}

// AuthorizeNonce returns an instruction to change the entity authorized to execute nonce instructions on the account
//
// The `Pubkey` parameter identifies the entity to authorize
func AuthorizeNonce(nonce ed25519.PublicKey) solana.Instruction {
	/// # Account references
	///   0. [WRITE, SIGNER] Nonce account
	data := make([]byte, 4+ed25519.PublicKeySize)
	binary.LittleEndian.PutUint32(data[:], commandAuthorizeNonceAccount)
	copy(data[4:], nonce[:])

	return solana.NewInstruction(
		ProgramKey[:],
		data,
		solana.NewAccountMeta(nonce, true),
	)
}
