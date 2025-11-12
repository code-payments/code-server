package common

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"

	"filippo.io/edwards25519"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/solana/token"
)

type Account struct {
	publicKey  *Key
	privateKey *Key // Optional
}

type TimelockAccounts struct {
	VaultOwner *Account

	State     *Account
	StateBump uint8

	Vault     *Account
	VaultBump uint8

	Unlock     *Account
	UnlockBump uint8

	VmDepositAccounts *VmDepositAccounts
	VmSwapAccounts    *VmSwapAccounts

	Vm   *Account
	Mint *Account
}

type VmDepositAccounts struct {
	VaultOwner *Account

	Pda     *Account
	PdaBump uint8

	Ata *Account

	Vm   *Account
	Mint *Account
}

type VmSwapAccounts struct {
	VaultOwner *Account

	Pda     *Account
	PdaBump uint8

	Ata *Account

	Vm   *Account
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

func (a *Account) ToTimelockVault(vmConfig *VmConfig) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	timelockAccounts, err := a.GetTimelockAccounts(vmConfig)
	if err != nil {
		return nil, err
	}
	return timelockAccounts.Vault, nil
}

func (a *Account) ToAssociatedTokenAccount(mint *Account) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	ata, err := token.GetAssociatedAccount(a.PublicKey().ToBytes(), mint.PublicKey().ToBytes())
	if err != nil {
		return nil, err
	}

	return NewAccountFromPublicKeyBytes(ata)
}

func (a *Account) ToVmDepositAta(vmConfig *VmConfig) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	vmDepositAccounts, err := a.GetVmDepositAccounts(vmConfig)
	if err != nil {
		return nil, err
	}
	return vmDepositAccounts.Ata, nil
}

func (a *Account) ToVmSwapAta(vmConfig *VmConfig) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	vmSwapAccounts, err := a.GetVmSwapAccounts(vmConfig)
	if err != nil {
		return nil, err
	}
	return vmSwapAccounts.Ata, nil
}

func (a *Account) GetTimelockAccounts(vmConfig *VmConfig) (*TimelockAccounts, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	stateAddress, stateBump, err := cvm.GetVirtualTimelockAccountAddress(&cvm.GetVirtualTimelockAccountAddressArgs{
		Mint:         vmConfig.Mint.PublicKey().ToBytes(),
		VmAuthority:  vmConfig.Authority.PublicKey().ToBytes(),
		Owner:        a.PublicKey().ToBytes(),
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
		Vm:                  vmConfig.Vm.publicKey.ToBytes(),
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

	vmDepositAccounts, err := a.GetVmDepositAccounts(vmConfig)
	if err != nil {
		return nil, errors.Wrap(err, "error getting vm deposit accounts")
	}

	vmSwapAccounts, err := a.GetVmSwapAccounts(vmConfig)
	if err != nil {
		return nil, errors.Wrap(err, "error getting vm swap accounts")
	}

	return &TimelockAccounts{
		VaultOwner: a,

		State:     stateAccount,
		StateBump: stateBump,

		Vault:     vaultAccount,
		VaultBump: vaultBump,

		Unlock:     unlockAccount,
		UnlockBump: unlockBump,

		VmDepositAccounts: vmDepositAccounts,
		VmSwapAccounts:    vmSwapAccounts,

		Vm:   vmConfig.Vm,
		Mint: vmConfig.Mint,
	}, nil
}

func (a *Account) GetVmDepositAccounts(vmConfig *VmConfig) (*VmDepositAccounts, error) {
	depositPdaAddress, depositPdaBump, err := cvm.GetVmDepositAddress(&cvm.GetVmDepositAddressArgs{
		Depositor: a.PublicKey().ToBytes(),
		Vm:        vmConfig.Vm.PublicKey().ToBytes(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error getting deposit pda address")
	}

	depositAtaAddress, err := token.GetAssociatedAccount(depositPdaAddress, vmConfig.Mint.PublicKey().ToBytes())
	if err != nil {
		return nil, errors.Wrap(err, "error getting deposit ata address")
	}

	depositPdaAccount, err := NewAccountFromPublicKeyBytes(depositPdaAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid deposit pda address")
	}

	depositAtaAccount, err := NewAccountFromPublicKeyBytes(depositAtaAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid deposit ata address")
	}

	return &VmDepositAccounts{
		Pda:     depositPdaAccount,
		PdaBump: depositPdaBump,

		Ata: depositAtaAccount,

		VaultOwner: a,

		Vm:   vmConfig.Vm,
		Mint: vmConfig.Mint,
	}, nil
}

func (a *Account) GetVmSwapAccounts(vmConfig *VmConfig) (*VmSwapAccounts, error) {
	swapPdaAddress, swapPdaBump, err := cvm.GetVmSwapAddress(&cvm.GetVmSwapAddressArgs{
		Swapper: a.PublicKey().ToBytes(),
		Vm:      vmConfig.Vm.PublicKey().ToBytes(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error getting swap pda address")
	}

	swapAtaAddress, err := token.GetAssociatedAccount(swapPdaAddress, vmConfig.Mint.PublicKey().ToBytes())
	if err != nil {
		return nil, errors.Wrap(err, "error getting swap ata address")
	}

	swapPdaAccount, err := NewAccountFromPublicKeyBytes(swapPdaAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid deposit pda address")
	}

	swapAtaAccount, err := NewAccountFromPublicKeyBytes(swapAtaAddress)
	if err != nil {
		return nil, errors.Wrap(err, "invalid deposit ata address")
	}

	return &VmSwapAccounts{
		Pda:     swapPdaAccount,
		PdaBump: swapPdaBump,

		Ata: swapAtaAccount,

		VaultOwner: a,

		Vm:   vmConfig.Vm,
		Mint: vmConfig.Mint,
	}, nil
}

func (a *Account) IsManagedByCode(ctx context.Context, data code_data.Provider) (bool, error) {
	timelockRecord, err := data.GetTimelockByVault(ctx, a.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "error getting cached timelock record")
	}

	return IsManagedByCode(ctx, timelockRecord), nil
}

func (a *Account) IsOnCurve() bool {
	return isOnCurve(a.PublicKey().ToBytes())
}

func (a *Account) Validate() error {
	if a == nil {
		return errors.New("account is nil")
	}

	if err := a.PublicKey().Validate(); err != nil {
		return errors.Wrap(err, "error validating public key")
	}

	if !a.PublicKey().IsPublic() {
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
	if !bytes.Equal(a.PublicKey().ToBytes(), expectedPublicKey) {
		return errors.New("private key doesn't map to public key")
	}

	return nil
}

func (a *Account) String() string {
	return a.PublicKey().ToBase58()
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
		Address: a.State.PublicKey().ToBase58(),
		Bump:    a.StateBump,

		VaultAddress: a.Vault.PublicKey().ToBase58(),
		VaultBump:    a.VaultBump,
		VaultOwner:   a.VaultOwner.PublicKey().ToBase58(),
		VaultState:   timelock_token_v1.StateUnknown,

		DepositPdaAddress: a.VmDepositAccounts.Pda.PublicKey().ToBase58(),
		DepositPdaBump:    a.VmDepositAccounts.PdaBump,

		UnlockAt: nil,

		Block: 0,
	}
}

// GetDBRecord fetches the equivalent timelock.Record for a TimelockAccounts from
// the DB
func (a *TimelockAccounts) GetDBRecord(ctx context.Context, data code_data.Provider) (*timelock.Record, error) {
	return data.GetTimelockByVault(ctx, a.Vault.PublicKey().ToBase58())
}

// GetInitializeInstruction gets a SystemTimelockInitInstruction instruction for a Timelock account
func (a *TimelockAccounts) GetInitializeInstruction(vmAuthority, memory *Account, accountIndex uint16) (solana.Instruction, error) {
	return cvm.NewInitTimelockInstruction(
		&cvm.InitTimelockInstructionAccounts{
			VmAuthority:         vmAuthority.PublicKey().ToBytes(),
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

// ValidateExternalTokenAccount validates an address is an external token account for the provided mint
func ValidateExternalTokenAccount(ctx context.Context, data code_data.Provider, tokenAccount, mintAccount *Account) (bool, string, error) {
	_, err := data.GetBlockchainTokenAccountInfo(ctx, tokenAccount.PublicKey().ToBase58(), mintAccount.PublicKey().ToBase58(), solana.CommitmentFinalized)
	switch err {
	case nil:
		// Double check there were no race conditions between other SubmitIntent
		// calls and scheduling. This would be highly unlikely to occur, but is a
		// safety precaution.
		_, err := data.GetAccountInfoByTokenAddress(ctx, tokenAccount.PublicKey().ToBase58())
		if err == nil {
			return false, fmt.Sprintf("%s is not an external account", tokenAccount.PublicKey().ToBase58()), nil
		} else if err == account.ErrAccountInfoNotFound {
			return true, "", nil
		} else if err != nil {
			return false, "", err
		}
		return true, "", nil
	case solana.ErrNoAccountInfo, token.ErrAccountNotFound:
		return false, fmt.Sprintf("%s doesn't exist on the blockchain", tokenAccount.PublicKey().ToBase58()), nil
	case token.ErrInvalidTokenAccount:
		return false, fmt.Sprintf("%s is not of %s mint", tokenAccount.PublicKey().ToBase58(), mintAccount.PublicKey().ToBase58()), nil
	default:
		// Unfortunate if Solana is down, but this only impacts withdraw flows,
		// and we need to guarantee this isn't going to something that's not
		// a core mint token acocunt.
		return false, "", err
	}
}

type LaunchpadCurrencyAccounts struct {
	Mint               *Account
	CurrencyConfig     *Account
	CurrencyConfigBump uint8
	LiquidityPool      *Account
	LiquidityPoolBump  uint8
	VaultBase          *Account
	VaultBaseBump      uint8
	VaultMint          *Account
	VaultMintBump      uint8
	FeesBase           *Account
	FeesMint           *Account
}

func GetLaunchpadCurrencyAccounts(metadataRecord *currency.MetadataRecord) (*LaunchpadCurrencyAccounts, error) {
	mint, err := NewAccountFromPublicKeyString(metadataRecord.Mint)
	if err != nil {
		return nil, err
	}
	currencyConfig, err := NewAccountFromPublicKeyString(metadataRecord.CurrencyConfig)
	if err != nil {
		return nil, err
	}
	liquidityPool, err := NewAccountFromPublicKeyString(metadataRecord.LiquidityPool)
	if err != nil {
		return nil, err
	}
	vaultBase, err := NewAccountFromPublicKeyString(metadataRecord.VaultCore)
	if err != nil {
		return nil, err
	}
	vaultMint, err := NewAccountFromPublicKeyString(metadataRecord.VaultMint)
	if err != nil {
		return nil, err
	}
	feesBase, err := NewAccountFromPublicKeyString(metadataRecord.FeesCore)
	if err != nil {
		return nil, err
	}
	feesMint, err := NewAccountFromPublicKeyString(metadataRecord.FeesMint)
	if err != nil {
		return nil, err
	}
	return &LaunchpadCurrencyAccounts{
		Mint:               mint,
		CurrencyConfig:     currencyConfig,
		CurrencyConfigBump: metadataRecord.CurrencyConfigBump,
		LiquidityPool:      liquidityPool,
		LiquidityPoolBump:  metadataRecord.LiquidityPoolBump,
		VaultBase:          vaultBase,
		VaultBaseBump:      metadataRecord.VaultCoreBump,
		VaultMint:          vaultMint,
		VaultMintBump:      metadataRecord.VaultMintBump,
		FeesBase:           feesBase,
		FeesMint:           feesMint,
	}, nil
}

func isOnCurve(pubKey ed25519.PublicKey) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}

	// Try to parse the public key as a point
	_, err := new(edwards25519.Point).SetBytes(pubKey)
	return err == nil
}
