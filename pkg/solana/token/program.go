package token

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"math"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// ProgramKey is the address of the token program that should be used.
//
// Current key: TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA
//
// todo: lock this in, or make configurable.
var ProgramKey = ed25519.PublicKey{6, 221, 246, 225, 215, 101, 161, 147, 217, 203, 225, 70, 206, 235, 121, 172, 28, 180, 133, 237, 95, 91, 55, 145, 58, 140, 245, 133, 126, 255, 0, 169}

type Command byte

const (
	// nolint:varcheck,deadcode,unused
	CommandInitializeMint Command = iota
	CommandInitializeAccount
	CommandInitializeMultisig
	CommandTransfer
	// nolint:varcheck,deadcode,unused
	CommandApprove
	// nolint:varcheck,deadcode,unused
	CommandRevoke
	CommandSetAuthority
	// nolint:varcheck,deadcode,unused
	CommandMintTo
	// nolint:varcheck,deadcode,unused
	CommandBurn
	CommandCloseAccount
	// nolint:varcheck,deadcode,unused
	CommandFreezeAccount
	// nolint:varcheck,deadcode,unused
	CommandThawAccount
	// nolint:varcheck,deadcode,unused
	CommandTransfer2
	// nolint:varcheck,deadcode,unused
	CommandApprove2
	// nolint:varcheck,deadcode,unused
	CommandMintTo2
	// nolint:varcheck,deadcode,unused
	CommandBurn2

	CommandUnknown = Command(math.MaxUint8)
)

const (
	// nolint:varcheck,deadcode,unused
	ErrorNotRentExempt solana.CustomError = iota
	// nolint:varcheck,deadcode,unused
	ErrorInsufficientFunds
	// nolint:varcheck,deadcode,unused
	ErrorInvalidMint
	// nolint:varcheck,deadcode,unused
	ErrorMintMismatch
	// nolint:varcheck,deadcode,unused
	ErrorOwnerMismatch
	// nolint:varcheck,deadcode,unused
	ErrorFixedSupply
	// nolint:varcheck,deadcode,unused
	ErrorAlreadyInUse
	// nolint:varcheck,deadcode,unused
	ErrorInvalidNumberOfProvidedSigners
	// nolint:varcheck,deadcode,unused
	ErrorInvalidNumberOfRequiredSigners
	// nolint:varcheck,deadcode,unused
	ErrorUninitializedState
	// nolint:varcheck,deadcode,unused
	ErrorNativeNotSupported
	// nolint:varcheck,deadcode,unused
	ErrorNonNativeHasBalance
	// nolint:varcheck,deadcode,unused
	ErrorInvalidInstruction
	// nolint:varcheck,deadcode,unused
	ErrorInvalidState
	// nolint:varcheck,deadcode,unused
	ErrorOverflow
	// nolint:varcheck,deadcode,unused
	ErrorAuthorityTypeNotSupported
	// nolint:varcheck,deadcode,unused
	ErrorMintCannotFreeze
	// nolint:varcheck,deadcode,unused
	ErrorAccountFrozen
	// nolint:varcheck,deadcode,unused
	ErrorMintDecimalsMismatch
)

func GetCommand(m solana.Message, index int) (Command, error) {
	if index >= len(m.Instructions) {
		return CommandUnknown, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return CommandUnknown, solana.ErrIncorrectProgram
	}
	if len(i.Data) == 0 {
		return CommandUnknown, errors.New("token instruction missing data")
	}

	return Command(i.Data[0]), nil
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L41-L55
func InitializeAccount(account, mint, owner ed25519.PublicKey) solana.Instruction {
	// Accounts expected by this instruction:
	//
	//   0. `[writable]`  The account to initialize.
	//   1. `[]` The mint this account will be associated with.
	//   2. `[]` The new account's owner/multisignature.
	//   3. `[]` Rent sysvar
	return solana.NewInstruction(
		ProgramKey,
		[]byte{byte(CommandInitializeAccount)},
		solana.NewAccountMeta(account, true),
		solana.NewReadonlyAccountMeta(mint, false),
		solana.NewReadonlyAccountMeta(owner, false),
		solana.NewReadonlyAccountMeta(system.RentSysVar, false),
	)
}

type DecompiledInitializeAccount struct {
	Account ed25519.PublicKey
	Mint    ed25519.PublicKey
	Owner   ed25519.PublicKey
}

func DecompileInitializeAccount(m solana.Message, index int) (*DecompiledInitializeAccount, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.Equal([]byte{byte(CommandInitializeAccount)}, i.Data) {
		return nil, solana.ErrIncorrectInstruction
	}
	if len(i.Accounts) != 4 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if !bytes.Equal(system.RentSysVar, m.Accounts[i.Accounts[3]]) {
		return nil, errors.Errorf("invalid rent program")
	}

	return &DecompiledInitializeAccount{
		Account: m.Accounts[i.Accounts[0]],
		Mint:    m.Accounts[i.Accounts[1]],
		Owner:   m.Accounts[i.Accounts[2]],
	}, nil
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L41-L55
func InitializeMultisig(account ed25519.PublicKey, requiredSigners byte, signers ...ed25519.PublicKey) solana.Instruction {
	// Accounts expected by this instruction:
	//
	//   0. `[writable]` The multisignature account to initialize.
	//   1. `[]` Rent sysvar
	//   2. ..2+N. `[]` The signer accounts, must equal to N where 1 <= N <= 11.
	accounts := make([]solana.AccountMeta, 2+len(signers))
	accounts[0] = solana.NewAccountMeta(account, false)
	accounts[1] = solana.NewReadonlyAccountMeta(system.RentSysVar, false)
	for i := 0; i < len(signers); i++ {
		accounts[i+2] = solana.NewReadonlyAccountMeta(signers[i], false)
	}

	return solana.NewInstruction(
		ProgramKey,
		[]byte{byte(CommandInitializeMultisig), requiredSigners},
		accounts...,
	)
}

type AuthorityType byte

const (
	AuthorityTypeMintTokens AuthorityType = iota
	AuthorityTypeFreezeAccount
	AuthorityTypeAccountHolder
	AuthorityTypeCloseAccount
)

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L128-L139
func SetAuthority(account, currentAuthority, newAuthority ed25519.PublicKey, authorityType AuthorityType) solana.Instruction {
	// Sets a new authority of a mint or account.
	//
	// Accounts expected by this instruction:
	//
	//   * Single authority
	//   0. `[writable]` The mint or account to change the authority of.
	//   1. `[signer]` The current authority of the mint or account.
	//
	//   * Multisignature authority
	//   0. `[writable]` The mint or account to change the authority of.
	//   1. `[]` The mint's or account's multisignature authority.
	//   2. ..2+M `[signer]` M signer accounts
	data := []byte{byte(CommandSetAuthority), byte(authorityType), 0}
	if len(newAuthority) > 0 {
		data[2] = 1
		data = append(data, newAuthority...)
	}

	return solana.NewInstruction(
		ProgramKey,
		data,
		solana.NewAccountMeta(account, false),
		solana.NewReadonlyAccountMeta(currentAuthority, true),
	)
}

func SetAuthorityMultisig(account, multisigOwner, newAuthority ed25519.PublicKey, authorityType AuthorityType, signers []ed25519.PublicKey) solana.Instruction {
	// Sets a new authority of a mint or account.
	//
	// Accounts expected by this instruction:
	//
	//   * Single authority
	//   0. `[writable]` The mint or account to change the authority of.
	//   1. `[signer]` The current authority of the mint or account.
	//
	//   * Multisignature authority
	//   0. `[writable]` The mint or account to change the authority of.
	//   1. `[]` The mint's or account's multisignature authority.
	//   2. ..2+M `[signer]` M signer accounts
	data := []byte{byte(CommandSetAuthority), byte(authorityType), 0}
	if len(newAuthority) > 0 {
		data[2] = 1
		data = append(data, newAuthority...)
	}

	accounts := make([]solana.AccountMeta, 2+len(signers))
	accounts[0] = solana.NewAccountMeta(account, false)
	accounts[1] = solana.NewReadonlyAccountMeta(multisigOwner, false)
	for i := 0; i < len(signers); i++ {
		accounts[2+i] = solana.NewReadonlyAccountMeta(signers[i], true)
	}

	return solana.NewInstruction(
		ProgramKey,
		data,
		accounts...,
	)
}

type DecompiledSetAuthority struct {
	Account          ed25519.PublicKey
	CurrentAuthority ed25519.PublicKey
	NewAuthority     ed25519.PublicKey
	Type             AuthorityType
}

func DecompileSetAuthority(m solana.Message, index int) (*DecompiledSetAuthority, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.HasPrefix(i.Data, []byte{byte(CommandSetAuthority)}) {
		return nil, solana.ErrIncorrectInstruction
	}
	if len(i.Accounts) < 2 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if len(i.Data) < 3 {
		return nil, errors.Errorf("invalid data size: %d (expect at least 3)", len(i.Data))
	}
	if i.Data[2] == 0 && len(i.Data) != 3 {
		return nil, errors.Errorf("invalid data size: %d (expect 3)", len(i.Data))
	}
	if i.Data[2] == 1 && len(i.Data) != 3+ed25519.PublicKeySize {
		return nil, errors.Errorf("invalid data size: %d (expect %d)", len(i.Data), 3+ed25519.PublicKeySize)
	}

	decompiled := &DecompiledSetAuthority{
		Account:          m.Accounts[i.Accounts[0]],
		CurrentAuthority: m.Accounts[i.Accounts[1]],
		Type:             AuthorityType(i.Data[1]),
	}

	if i.Data[2] == 1 {
		decompiled.NewAuthority = i.Data[3 : 3+ed25519.PublicKeySize]
	}

	return decompiled, nil
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L76-L91
func Transfer(source, dest, owner ed25519.PublicKey, amount uint64) solana.Instruction {
	// Accounts expected by this instruction:
	//
	//   * Single owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[writable]` The destination account.
	//   2. `[signer]` The source account's owner/delegate.
	//
	//   * Multisignature owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[writable]` The destination account.
	//   2. `[]` The source account's multisignature owner/delegate.
	//   3. ..3+M `[signer]` M signer accounts.
	data := make([]byte, 1+8)
	data[0] = byte(CommandTransfer)
	binary.LittleEndian.PutUint64(data[1:], amount)

	return solana.NewInstruction(
		ProgramKey,
		data,
		solana.NewAccountMeta(source, false),
		solana.NewAccountMeta(dest, false),
		solana.NewReadonlyAccountMeta(owner, true),
	)
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L230-L252
func Transfer2(source, mint, dest, owner ed25519.PublicKey, amount uint64, decimals byte) solana.Instruction {
	// Accounts expected by this instruction:
	//
	//   * Single owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[]` The token mint.
	//   2. `[writable]` The destination account.
	//   3. `[signer]` The source account's owner/delegate.
	//
	//   * Multisignature owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[]` The token mint.
	//   2. `[writable]` The destination account.
	//   3. `[]` The source account's multisignature owner/delegate.
	//   4. ..4+M `[signer]` M signer accounts.
	data := make([]byte, 1+8+1)
	data[0] = byte(CommandTransfer2)
	binary.LittleEndian.PutUint64(data[1:], amount)
	data[9] = decimals

	return solana.NewInstruction(
		ProgramKey,
		data,
		solana.NewAccountMeta(source, false),
		solana.NewReadonlyAccountMeta(mint, false),
		solana.NewAccountMeta(dest, false),
		solana.NewReadonlyAccountMeta(owner, true),
	)
}

func TransferMultisig(source, dest, multisigOwner ed25519.PublicKey, amount uint64, signers ...ed25519.PublicKey) solana.Instruction {
	// Accounts expected by this instruction:
	//
	//   * Single owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[writable]` The destination account.
	//   2. `[signer]` The source account's owner/delegate.
	//
	//   * Multisignature owner/delegate
	//   0. `[writable]` The source account.
	//   1. `[writable]` The destination account.
	//   2. `[]` The source account's multisignature owner/delegate.
	//   3. ..3+M `[signer]` M signer accounts.
	data := make([]byte, 1+8)
	data[0] = byte(CommandTransfer)
	binary.LittleEndian.PutUint64(data[1:], amount)

	accounts := make([]solana.AccountMeta, 3+len(signers))
	accounts[0] = solana.NewAccountMeta(source, false)
	accounts[1] = solana.NewAccountMeta(dest, false)
	accounts[2] = solana.NewReadonlyAccountMeta(multisigOwner, false)
	for i := 0; i < len(signers); i++ {
		accounts[3+i] = solana.NewReadonlyAccountMeta(signers[i], true)
	}

	return solana.NewInstruction(
		ProgramKey,
		data,
		accounts...,
	)
}

type DecompiledTransfer struct {
	Source      ed25519.PublicKey
	Destination ed25519.PublicKey
	Owner       ed25519.PublicKey
	Amount      uint64
}

func DecompileTransfer(m solana.Message, index int) (*DecompiledTransfer, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.HasPrefix(i.Data, []byte{byte(CommandTransfer)}) {
		return nil, solana.ErrIncorrectInstruction
	}
	// note: we do < 3 instead of != 3 in order to support multisig cases.
	if len(i.Accounts) < 3 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if len(i.Data) != 9 {
		return nil, errors.Errorf("invalid instruction data size: %d", len(i.Data))
	}

	v := &DecompiledTransfer{
		Source:      m.Accounts[i.Accounts[0]],
		Destination: m.Accounts[i.Accounts[1]],
		Owner:       m.Accounts[i.Accounts[2]],
	}
	v.Amount = binary.LittleEndian.Uint64(i.Data[1:])
	return v, nil
}

type DecompiledTransfer2 struct {
	Source      ed25519.PublicKey
	Mint        ed25519.PublicKey
	Destination ed25519.PublicKey
	Owner       ed25519.PublicKey
	Amount      uint64
	Decimals    byte
}

func DecompileTransfer2(m solana.Message, index int) (*DecompiledTransfer2, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.HasPrefix(i.Data, []byte{byte(CommandTransfer2)}) {
		return nil, solana.ErrIncorrectInstruction
	}
	// note: we do < 4 instead of != 4 in order to support multisig cases.
	if len(i.Accounts) < 4 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}
	if len(i.Data) != 10 {
		return nil, errors.Errorf("invalid instruction data size: %d", len(i.Data))
	}

	v := &DecompiledTransfer2{
		Source:      m.Accounts[i.Accounts[0]],
		Mint:        m.Accounts[i.Accounts[1]],
		Destination: m.Accounts[i.Accounts[2]],
		Owner:       m.Accounts[i.Accounts[3]],
	}
	v.Amount = binary.LittleEndian.Uint64(i.Data[1:9])
	v.Decimals = i.Data[9]
	return v, nil
}

// Reference: https://github.com/solana-labs/solana-program-library/blob/b011698251981b5a12088acba18fad1d41c3719a/token/program/src/instruction.rs#L183-L197
func CloseAccount(account, dest, owner ed25519.PublicKey) solana.Instruction {
	// Close an account by transferring all its SOL to the destination account.
	// Non-native accounts may only be closed if its token amount is zero.
	//
	// Accounts expected by this instruction:
	//
	//   * Single owner
	//   0. `[writable]` The account to close.
	//   1. `[writable]` The destination account.
	//   2. `[signer]` The account's owner.
	//
	//   * Multisignature owner
	//   0. `[writable]` The account to close.
	//   1. `[writable]` The destination account.
	//   2. `[]` The account's multisignature owner.
	//   3. ..3+M `[signer]` M signer accounts.
	return solana.NewInstruction(
		ProgramKey,
		[]byte{byte(CommandCloseAccount)},
		solana.NewAccountMeta(account, false),
		solana.NewAccountMeta(dest, false),
		solana.NewReadonlyAccountMeta(owner, true),
	)
}

type DecompiledCloseAccount struct {
	Account     ed25519.PublicKey
	Destination ed25519.PublicKey
	Owner       ed25519.PublicKey
}

func DecompileCloseAccount(m solana.Message, index int) (*DecompiledCloseAccount, error) {
	if index >= len(m.Instructions) {
		return nil, errors.Errorf("instruction doesn't exist at %d", index)
	}

	i := m.Instructions[index]

	if !bytes.Equal(m.Accounts[i.ProgramIndex], ProgramKey) {
		return nil, solana.ErrIncorrectProgram
	}
	if !bytes.Equal(i.Data, []byte{byte(CommandCloseAccount)}) {
		return nil, solana.ErrIncorrectInstruction
	}
	// note: we do < 3 instead of != 3 in order to support multisig cases.
	if len(i.Accounts) < 3 {
		return nil, errors.Errorf("invalid number of accounts: %d", len(i.Accounts))
	}

	v := &DecompiledCloseAccount{
		Account:     m.Accounts[i.Accounts[0]],
		Destination: m.Accounts[i.Accounts[1]],
		Owner:       m.Accounts[i.Accounts[2]],
	}
	return v, nil
}
