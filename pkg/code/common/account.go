package common

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/solana/token"
)

var (
	ErrNoPrivacyMigration2022 = errors.New("no privacy migration 2022 for owner")
)

type Account struct {
	publicKey  *Key
	privateKey *Key // Optional
}

type TimelockAccounts struct {
	Vm *Account

	State     *Account
	StateBump uint8

	Vault     *Account
	VaultBump uint8

	Unlock     *Account
	UnlockBump uint8

	VaultOwner *Account

	Mint *Account
}

type AccountRecords struct {
	General  *account.Record
	Timelock *timelock.Record
}

func NewAccountFromPublicKey(publicKey *Key) (*Account, error) {
	account := &Account{
		publicKey: publicKey,
	}

	if err := account.Validate(); err != nil {
		return nil, err
	}
	return account, nil
}

func NewAccountFromPublicKeyBytes(publicKey []byte) (*Account, error) {
	key, err := NewKeyFromBytes(publicKey)
	if err != nil {
		return nil, err
	}

	return NewAccountFromPublicKey(key)
}

func NewAccountFromPublicKeyString(publicKey string) (*Account, error) {
	key, err := NewKeyFromString(publicKey)
	if err != nil {
		return nil, err
	}

	return NewAccountFromPublicKey(key)
}

func NewAccountFromPrivateKey(privateKey *Key) (*Account, error) {
	publicKeyBytes := ed25519.PrivateKey(privateKey.ToBytes()).Public().(ed25519.PublicKey)
	publicKey, err := NewKeyFromBytes(publicKeyBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error creating public key from private key")
	}

	account := &Account{
		publicKey:  publicKey,
		privateKey: privateKey,
	}

	if err := account.Validate(); err != nil {
		return nil, err
	}
	return account, nil
}

func NewAccountFromPrivateKeyBytes(publicKey []byte) (*Account, error) {
	key, err := NewKeyFromBytes(publicKey)
	if err != nil {
		return nil, err
	}

	return NewAccountFromPrivateKey(key)
}

func NewAccountFromPrivateKeyString(privateKey string) (*Account, error) {
	key, err := NewKeyFromString(privateKey)
	if err != nil {
		return nil, err
	}

	return NewAccountFromPrivateKey(key)
}

func NewAccountFromProto(proto *commonpb.SolanaAccountId) (*Account, error) {
	publicKey, err := NewKeyFromBytes(proto.Value)
	if err != nil {
		return nil, err
	}

	return NewAccountFromPublicKey(publicKey)
}

func NewRandomAccount() (*Account, error) {
	key, err := NewRandomKey()
	if err != nil {
		return nil, err
	}

	account, err := NewAccountFromPrivateKey(key)
	if err != nil {
		return nil, errors.Wrap(err, "invalid account")
	}

	return account, nil
}

func (a *Account) PublicKey() *Key {
	return a.publicKey
}

func (a *Account) PrivateKey() *Key {
	return a.privateKey
}

func (a *Account) ToProto() *commonpb.SolanaAccountId {
	return &commonpb.SolanaAccountId{
		Value: a.publicKey.ToBytes(),
	}
}

func (a *Account) Sign(message []byte) ([]byte, error) {
	if a.privateKey == nil {
		return nil, errors.New("private key not available")
	}

	signature := ed25519.Sign(a.privateKey.ToBytes(), message)
	return signature, nil
}

func (a *Account) ToTimelockVault(vm, mint *Account) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	timelockAccounts, err := a.GetTimelockAccounts(vm, mint)
	if err != nil {
		return nil, err
	}
	return timelockAccounts.Vault, nil
}

func (a *Account) ToAssociatedTokenAccount(mint *Account) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	ata, err := token.GetAssociatedAccount(a.publicKey.ToBytes(), mint.publicKey.ToBytes())
	if err != nil {
		return nil, err
	}

	return NewAccountFromPublicKeyBytes(ata)
}

func (a *Account) GetTimelockAccounts(vm, mint *Account) (*TimelockAccounts, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	stateAddress, stateBump, err := cvm.GetVirtualTimelockAccountAddress(&cvm.GetVirtualTimelockAccountAddressArgs{
		Mint:         mint.publicKey.ToBytes(),
		VmAuthority:  GetSubsidizer().publicKey.ToBytes(),
		Owner:        a.publicKey.ToBytes(),
		LockDuration: timelock_token_v1.DefaultNumDaysLocked,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error getting timelock state address")
	}

	vaultAddress, vaultBump, err := cvm.GetVirtualTimelockVaultAddress(&cvm.GetVirtualTimelockVaultAddressArgs{
		VirtualTimelock: stateAddress,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error getting vault address")
	}

	unlockAddress, unlockBump, err := cvm.GetVmUnlockStateAccountAddress(&cvm.GetVmUnlockStateAccountAddressArgs{
		VirtualAccountOwner: a.publicKey.ToBytes(),
		VirtualAccount:      stateAddress,
		Vm:                  vm.publicKey.ToBytes(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error getting unlock address")
	}

	stateAccount, err := NewAccountFromPublicKeyBytes(stateAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid state address")
	}

	vaultAccount, err := NewAccountFromPublicKeyBytes(vaultAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid vault address")
	}

	unlockAccount, err := NewAccountFromPublicKeyBytes(unlockAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid unlock address")
	}

	return &TimelockAccounts{
		Vm: vm,

		VaultOwner: a,

		State:     stateAccount,
		StateBump: stateBump,

		Vault:     vaultAccount,
		VaultBump: vaultBump,

		Unlock:     unlockAccount,
		UnlockBump: unlockBump,

		Mint: mint,
	}, nil
}

func (a *Account) IsManagedByCode(ctx context.Context, data code_data.Provider) (bool, error) {
	timelockRecord, err := data.GetTimelockByVault(ctx, a.publicKey.ToBase58())
	if err == timelock.ErrTimelockNotFound {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "error getting cached timelock record")
	}

	return IsManagedByCode(ctx, timelockRecord), nil
}

func (a *Account) Validate() error {
	if a == nil {
		return errors.New("account is nil")
	}

	if err := a.publicKey.Validate(); err != nil {
		return errors.Wrap(err, "error validating public key")
	}

	if !a.publicKey.IsPublic() {
		return errors.New("public key isn't public")
	}

	// Private keys are optional
	if a.privateKey == nil {
		return nil
	}

	if err := a.privateKey.Validate(); err != nil {
		return errors.Wrap(err, "error validating private key")
	}

	if a.privateKey.IsPublic() {
		return errors.New("private key isn't private")
	}

	expectedPublicKey := ed25519.PrivateKey(a.privateKey.ToBytes()).Public().(ed25519.PublicKey)
	if !bytes.Equal(a.publicKey.ToBytes(), expectedPublicKey) {
		return errors.New("private key doesn't map to public key")
	}

	return nil
}

func (a *Account) String() string {
	return a.publicKey.ToBase58()
}

func (r *AccountRecords) IsManagedByCode(ctx context.Context) bool {
	if !r.IsTimelock() {
		return false
	}
	return IsManagedByCode(ctx, r.Timelock)
}

func (r *AccountRecords) IsTimelock() bool {
	return r.Timelock != nil
}

func IsManagedByCode(ctx context.Context, timelockRecord *timelock.Record) bool {
	// todo: check if the VM is managed by Code

	// todo: We don't support unlocking timelock accounts and leaving the open,
	//       but we may need to scan the intents system for a RevokeWithAuthority
	//       instruction as another negative case for this function.
	return timelockRecord.IsLocked()
}

// ToDBRecord transforms the TimelockAccounts struct to a default timelock.Record
func (a *TimelockAccounts) ToDBRecord() *timelock.Record {
	return &timelock.Record{
		Address: a.State.publicKey.ToBase58(),
		Bump:    a.StateBump,

		VaultAddress: a.Vault.publicKey.ToBase58(),
		VaultBump:    a.VaultBump,
		VaultOwner:   a.VaultOwner.publicKey.ToBase58(),
		VaultState:   timelock_token_v1.StateUnknown,

		UnlockAt: nil,

		Block: 0,
	}
}

// GetDBRecord fetches the equivalent timelock.Record for a TimelockAccounts from
// the DB
func (a *TimelockAccounts) GetDBRecord(ctx context.Context, data code_data.Provider) (*timelock.Record, error) {
	return data.GetTimelockByVault(ctx, a.Vault.publicKey.ToBase58())
}

// GetInitializeInstruction gets a SystemTimelockInitInstruction instruction for a timelock account
func (a *TimelockAccounts) GetInitializeInstruction(memory *Account, accountIndex uint16) (solana.Instruction, error) {
	return cvm.NewInitTimelockInstruction(
		&cvm.InitTimelockInstructionAccounts{
			VmAuthority:         GetSubsidizer().publicKey.ToBytes(),
			Vm:                  a.Vm.PublicKey().ToBytes(),
			VmMemory:            memory.PublicKey().ToBytes(),
			VirtualAccountOwner: a.VaultOwner.PublicKey().ToBytes(),
		},
		&cvm.InitTimelockInstructionArgs{
			AccountIndex:        accountIndex,
			VirtualTimelockBump: a.StateBump,
			VirtualVaultBump:    a.VaultBump,
			VmUnlockPdaBump:     a.UnlockBump,
		},
	), nil
}

// GetTransferWithAuthorityInstruction gets a TransferWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetTransferWithAuthorityInstruction(destination *Account, quarks uint64) (solana.Instruction, error) {
	if err := destination.Validate(); err != nil {
		return solana.Instruction{}, err
	}

	if quarks == 0 {
		return solana.Instruction{}, errors.New("quarks must be positive")
	}

	return timelock_token_v1.NewTransferWithAuthorityInstruction(
		&timelock_token_v1.TransferWithAuthorityInstructionAccounts{
			Timelock:      a.State.publicKey.ToBytes(),
			Vault:         a.Vault.publicKey.ToBytes(),
			VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
			TimeAuthority: GetSubsidizer().publicKey.ToBytes(),
			Destination:   destination.publicKey.ToBytes(),
			Payer:         GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.TransferWithAuthorityInstructionArgs{
			TimelockBump: a.StateBump,
			Amount:       quarks,
		},
	).ToLegacyInstruction(), nil
}

// GetWithdrawInstruction gets a Withdraw instruction for a timelock account
func (a *TimelockAccounts) GetWithdrawInstruction(destination *Account) (solana.Instruction, error) {
	if err := destination.Validate(); err != nil {
		return solana.Instruction{}, err
	}

	return timelock_token_v1.NewWithdrawInstruction(
		&timelock_token_v1.WithdrawInstructionAccounts{
			Timelock:    a.State.publicKey.ToBytes(),
			Vault:       a.Vault.publicKey.ToBytes(),
			VaultOwner:  a.VaultOwner.publicKey.ToBytes(),
			Destination: destination.publicKey.ToBytes(),
			Payer:       GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.WithdrawInstructionArgs{
			TimelockBump: a.StateBump,
		},
	).ToLegacyInstruction(), nil
}

// GetBurnDustWithAuthorityInstruction gets a BurnDustWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetBurnDustWithAuthorityInstruction(maxQuarks uint64) (solana.Instruction, error) {
	if maxQuarks == 0 {
		return solana.Instruction{}, errors.New("max quarks must be positive")
	}

	return timelock_token_v1.NewBurnDustWithAuthorityInstruction(
		&timelock_token_v1.BurnDustWithAuthorityInstructionAccounts{
			Timelock:      a.State.publicKey.ToBytes(),
			Vault:         a.Vault.publicKey.ToBytes(),
			VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
			TimeAuthority: GetSubsidizer().publicKey.ToBytes(),
			Mint:          a.Mint.publicKey.ToBytes(),
			Payer:         GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.BurnDustWithAuthorityInstructionArgs{
			TimelockBump: a.StateBump,
			MaxAmount:    maxQuarks,
		},
	).ToLegacyInstruction(), nil
}

// GetRevokeLockWithAuthorityInstruction gets a RevokeLockWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetRevokeLockWithAuthorityInstruction() (solana.Instruction, error) {
	return timelock_token_v1.NewRevokeLockWithAuthorityInstruction(
		&timelock_token_v1.RevokeLockWithAuthorityInstructionAccounts{
			Timelock:      a.State.publicKey.ToBytes(),
			Vault:         a.Vault.publicKey.ToBytes(),
			TimeAuthority: GetSubsidizer().publicKey.ToBytes(),
			Payer:         GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.RevokeLockWithAuthorityInstructionArgs{
			TimelockBump: a.StateBump,
		},
	).ToLegacyInstruction(), nil
}

// GetDeactivateInstruction gets a Deactivate instruction for a timelock account
func (a *TimelockAccounts) GetDeactivateInstruction() (solana.Instruction, error) {
	return timelock_token_v1.NewDeactivateInstruction(
		&timelock_token_v1.DeactivateInstructionAccounts{
			Timelock:   a.State.publicKey.ToBytes(),
			VaultOwner: a.VaultOwner.publicKey.ToBytes(),
			Payer:      GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.DeactivateInstructionArgs{
			TimelockBump: a.StateBump,
		},
	).ToLegacyInstruction(), nil
}

// GetCloseAccountsInstruction gets a CloseAccounts instruction for a timelock account
func (a *TimelockAccounts) GetCloseAccountsInstruction() (solana.Instruction, error) {
	return timelock_token_v1.NewCloseAccountsInstruction(
		&timelock_token_v1.CloseAccountsInstructionAccounts{
			Timelock:       a.State.publicKey.ToBytes(),
			Vault:          a.Vault.publicKey.ToBytes(),
			CloseAuthority: GetSubsidizer().publicKey.ToBytes(),
			Payer:          GetSubsidizer().publicKey.ToBytes(),
		},
		&timelock_token_v1.CloseAccountsInstructionArgs{
			TimelockBump: a.StateBump,
		},
	).ToLegacyInstruction(), nil
}

// ValidateExternalKinTokenAccount validates an address is an external Kin token account
func ValidateExternalKinTokenAccount(ctx context.Context, data code_data.Provider, tokenAccount *Account) (bool, string, error) {
	_, err := data.GetBlockchainTokenAccountInfo(ctx, tokenAccount.publicKey.ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		// Double check there were no race conditions between other SubmitIntent
		// calls and scheduling. This would be highly unlikely to occur, but is a
		// safety precaution.
		_, err := data.GetAccountInfoByTokenAddress(ctx, tokenAccount.publicKey.ToBase58())
		if err == nil {
			return false, fmt.Sprintf("%s is not an external account", tokenAccount.publicKey.ToBase58()), nil
		} else if err == account.ErrAccountInfoNotFound {
			return true, "", nil
		} else if err != nil {
			return false, "", err
		}
		return true, "", nil
	case solana.ErrNoAccountInfo, token.ErrAccountNotFound:
		return false, fmt.Sprintf("%s doesn't exist on the blockchain", tokenAccount.publicKey.ToBase58()), nil
	case token.ErrInvalidTokenAccount:
		return false, fmt.Sprintf("%s is not a kin token account", tokenAccount.publicKey.ToBase58()), nil
	default:
		// Unfortunate if Solana is down, but this only impacts withdraw flows,
		// and we need to guarantee this isn't going to something that's not
		// a Kin token acocunt.
		return false, "", err
	}
}
