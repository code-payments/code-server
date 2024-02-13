package common

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
	timelock_token_legacy "github.com/code-payments/code-server/pkg/solana/timelock/legacy_2022"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/solana/token"
)

const (
	dangerousTimelockAccessCountMetricName = "Account/dangerous_timelock_access_count"
)

var (
	// defaultTimelockNonceAccount is the default nonce account used to derive
	// legacy 2022 timelock PDAs.
	//
	// Important Note: Be very careful changing this value, as it will completely
	// change timelock PDAs.
	defaultTimelockNonceAccount *Account
)

var (
	ErrNoPrivacyMigration2022 = errors.New("no privacy migration 2022 for owner")
)

func init() {
	var err error
	defaultTimelockNonceAccount, err = NewAccountFromPublicKeyBytes(make([]byte, ed25519.PublicKeySize))
	if err != nil {
		panic(err)
	}
}

type Account struct {
	publicKey  *Key
	privateKey *Key // Optional
}

type TimelockAccounts struct {
	DataVersion timelock_token_v1.TimelockDataVersion

	State     *Account
	StateBump uint8

	Vault     *Account
	VaultBump uint8

	VaultOwner *Account

	TimeAuthority  *Account
	CloseAuthority *Account

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

func (a *Account) ToTimelockVault(dataVersion timelock_token_v1.TimelockDataVersion, mint *Account) (*Account, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	timelockAccounts, err := a.GetTimelockAccounts(dataVersion, mint)
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

func (a *Account) GetTimelockAccounts(dataVersion timelock_token_v1.TimelockDataVersion, mint *Account) (*TimelockAccounts, error) {
	if err := a.Validate(); err != nil {
		return nil, errors.Wrap(err, "error validating owner account")
	}

	var timelockAccounts *TimelockAccounts
	switch dataVersion {
	case timelock_token_v1.DataVersion1:
		stateAddress, stateBump, err := timelock_token_v1.GetStateAddress(&timelock_token_v1.GetStateAddressArgs{
			Mint:          mint.publicKey.ToBytes(),
			TimeAuthority: GetSubsidizer().publicKey.ToBytes(),
			VaultOwner:    a.publicKey.ToBytes(),
			NumDaysLocked: timelock_token_v1.DefaultNumDaysLocked,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error getting timelock state address")
		}

		vaultAddress, vaultBump, err := timelock_token_v1.GetVaultAddress(&timelock_token_v1.GetVaultAddressArgs{
			State:       stateAddress,
			DataVersion: timelock_token_v1.DataVersion1,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error getting vault address")
		}

		stateAccount, err := NewAccountFromPublicKeyBytes(stateAddress)
		if err != nil {
			return nil, errors.Wrap(err, "invalid state address")
		}

		vaultAccount, err := NewAccountFromPublicKeyBytes(vaultAddress)
		if err != nil {
			return nil, errors.Wrap(err, "invalid vault address")
		}

		timelockAccounts = &TimelockAccounts{
			State:     stateAccount,
			StateBump: stateBump,

			Vault:     vaultAccount,
			VaultBump: vaultBump,

			Mint: mint,
		}
	case timelock_token_v1.DataVersionLegacy:
		stateAddress, stateBump, err := timelock_token_legacy.GetStateAddress(&timelock_token_legacy.GetStateAddressArgs{
			Mint:           mint.publicKey.ToBytes(),
			TimeAuthority:  GetSubsidizer().publicKey.ToBytes(),
			Nonce:          defaultTimelockNonceAccount.publicKey.ToBytes(),
			VaultOwner:     a.publicKey.ToBytes(),
			UnlockDuration: timelock_token_legacy.DefaultUnlockDuration,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error getting timelock state address")
		}

		vaultAddress, vaultBump, err := timelock_token_legacy.GetVaultAddress(&timelock_token_legacy.GetVaultAddressArgs{
			State: stateAddress,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error getting vault address")
		}

		stateAccount, err := NewAccountFromPublicKeyBytes(stateAddress)
		if err != nil {
			return nil, errors.Wrap(err, "invalid state address")
		}

		vaultAccount, err := NewAccountFromPublicKeyBytes(vaultAddress)
		if err != nil {
			return nil, errors.Wrap(err, "invalid vault address")
		}

		timelockAccounts = &TimelockAccounts{
			State:     stateAccount,
			StateBump: stateBump,

			Vault:     vaultAccount,
			VaultBump: vaultBump,

			Mint: mint,
		}
	default:
		return nil, errors.New("unsupported data version")
	}

	timelockAccounts.DataVersion = dataVersion
	timelockAccounts.VaultOwner = a
	timelockAccounts.TimeAuthority = GetSubsidizer()
	timelockAccounts.CloseAuthority = GetSubsidizer()
	return timelockAccounts, nil
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
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"type":   "common/account",
		"method": "IsManagedByCode",
		"vault":  timelockRecord.VaultAddress,
	})

	// This should never happen, but is a precautionary check.
	if timelockRecord.DataVersion == timelock_token_v1.DataVersionClosed {
		metrics.RecordCount(ctx, dangerousTimelockAccessCountMetricName, 1)
		log.Warn("detected a dangerous timelock account with a closed data version")
		return false
	}

	// This should never happen, but is a precautionary check.
	if timelockRecord.TimeAuthority != GetSubsidizer().publicKey.ToBase58() {
		metrics.RecordCount(ctx, dangerousTimelockAccessCountMetricName, 1)
		log.Warn("detected a dangerous timelock account with a time authority that's not Code")
		return false
	}

	// This should never happen, but is a precautionary check.
	if timelockRecord.CloseAuthority != GetSubsidizer().publicKey.ToBase58() {
		metrics.RecordCount(ctx, dangerousTimelockAccessCountMetricName, 1)
		log.Warn("detected a dangerous timelock account with a close authority that's not Code")
		return false
	}

	// todo: We don't support unlocking timelock accounts and leaving the open,
	//       but we may need to scan the intents system for a RevokeWithAuthority
	//       instruction as another negative case for this function.
	return timelockRecord.IsLocked()
}

// ToDBRecord transforms the TimelockAccounts struct to a default timelock.Record
func (a *TimelockAccounts) ToDBRecord() *timelock.Record {
	return &timelock.Record{
		DataVersion: a.DataVersion,

		Address: a.State.publicKey.ToBase58(),
		Bump:    a.StateBump,

		VaultAddress: a.Vault.publicKey.ToBase58(),
		VaultBump:    a.VaultBump,
		VaultOwner:   a.VaultOwner.publicKey.ToBase58(),
		VaultState:   timelock_token_v1.StateUnknown,

		TimeAuthority:  a.TimeAuthority.publicKey.ToBase58(),
		CloseAuthority: a.CloseAuthority.publicKey.ToBase58(),

		Mint: a.Mint.publicKey.ToBase58(),

		NumDaysLocked: timelock_token_v1.DefaultNumDaysLocked,
		UnlockAt:      nil,

		Block: 0,
	}
}

// GetDBRecord fetches the equivalent timelock.Record for a TimelockAccounts from
// the DB
func (a *TimelockAccounts) GetDBRecord(ctx context.Context, data code_data.Provider) (*timelock.Record, error) {
	return data.GetTimelockByVault(ctx, a.Vault.publicKey.ToBase58())
}

// GetInitializeInstruction gets an Initialize instruction for a timelock account
func (a *TimelockAccounts) GetInitializeInstruction() (solana.Instruction, error) {
	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
		return timelock_token_v1.NewInitializeInstruction(
			&timelock_token_v1.InitializeInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
				Mint:          a.Mint.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Payer:         a.CloseAuthority.publicKey.ToBytes(),
			},
			&timelock_token_v1.InitializeInstructionArgs{
				NumDaysLocked: timelock_token_v1.DefaultNumDaysLocked,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetTransferWithAuthorityInstruction gets a TransferWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetTransferWithAuthorityInstruction(destination *Account, quarks uint64) (solana.Instruction, error) {
	if err := destination.Validate(); err != nil {
		return solana.Instruction{}, err
	}

	if quarks == 0 {
		return solana.Instruction{}, errors.New("quarks must be positive")
	}

	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
		return timelock_token_v1.NewTransferWithAuthorityInstruction(
			&timelock_token_v1.TransferWithAuthorityInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Destination:   destination.publicKey.ToBytes(),
				Payer:         GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_v1.TransferWithAuthorityInstructionArgs{
				TimelockBump: a.StateBump,
				Amount:       quarks,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetWithdrawInstruction gets a Withdraw instruction for a timelock account
func (a *TimelockAccounts) GetWithdrawInstruction(destination *Account) (solana.Instruction, error) {
	if err := destination.Validate(); err != nil {
		return solana.Instruction{}, err
	}

	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
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
	case timelock_token_v1.DataVersionLegacy:
		return timelock_token_legacy.NewWithdrawInstruction(
			&timelock_token_legacy.WithdrawInstructionAccounts{
				Timelock:    a.State.publicKey.ToBytes(),
				Vault:       a.Vault.publicKey.ToBytes(),
				VaultOwner:  a.VaultOwner.publicKey.ToBytes(),
				Destination: destination.publicKey.ToBytes(),
				Payer:       GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_legacy.WithdrawInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetBurnDustWithAuthorityInstruction gets a BurnDustWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetBurnDustWithAuthorityInstruction(maxQuarks uint64) (solana.Instruction, error) {
	if maxQuarks == 0 {
		return solana.Instruction{}, errors.New("max quarks must be positive")
	}

	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
		return timelock_token_v1.NewBurnDustWithAuthorityInstruction(
			&timelock_token_v1.BurnDustWithAuthorityInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Mint:          a.Mint.publicKey.ToBytes(),
				Payer:         GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_v1.BurnDustWithAuthorityInstructionArgs{
				TimelockBump: a.StateBump,
				MaxAmount:    maxQuarks,
			},
		).ToLegacyInstruction(), nil
	case timelock_token_v1.DataVersionLegacy:
		return timelock_token_legacy.NewBurnDustWithAuthorityInstruction(
			&timelock_token_legacy.BurnDustWithAuthorityInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				VaultOwner:    a.VaultOwner.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Mint:          a.Mint.publicKey.ToBytes(),
				Payer:         GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_legacy.BurnDustWithAuthorityInstructionArgs{
				TimelockBump: a.StateBump,
				MaxAmount:    maxQuarks,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetRevokeLockWithAuthorityInstruction gets a RevokeLockWithAuthority instruction for a timelock account
func (a *TimelockAccounts) GetRevokeLockWithAuthorityInstruction() (solana.Instruction, error) {
	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
		return timelock_token_v1.NewRevokeLockWithAuthorityInstruction(
			&timelock_token_v1.RevokeLockWithAuthorityInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Payer:         GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_v1.RevokeLockWithAuthorityInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	case timelock_token_v1.DataVersionLegacy:
		return timelock_token_legacy.NewRevokeLockWithAuthorityInstruction(
			&timelock_token_legacy.RevokeLockWithAuthorityInstructionAccounts{
				Timelock:      a.State.publicKey.ToBytes(),
				Vault:         a.Vault.publicKey.ToBytes(),
				TimeAuthority: a.TimeAuthority.publicKey.ToBytes(),
				Payer:         GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_legacy.RevokeLockWithAuthorityInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetDeactivateInstruction gets a Deactivate instruction for a timelock account
func (a *TimelockAccounts) GetDeactivateInstruction() (solana.Instruction, error) {
	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
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
	case timelock_token_v1.DataVersionLegacy:
		return timelock_token_legacy.NewDeactivateInstruction(
			&timelock_token_legacy.DeactivateInstructionAccounts{
				Timelock:   a.State.publicKey.ToBytes(),
				VaultOwner: a.VaultOwner.publicKey.ToBytes(),
				Payer:      GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_legacy.DeactivateInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
}

// GetCloseAccountsInstruction gets a CloseAccounts instruction for a timelock account
func (a *TimelockAccounts) GetCloseAccountsInstruction() (solana.Instruction, error) {
	switch a.DataVersion {
	case timelock_token_v1.DataVersion1:
		return timelock_token_v1.NewCloseAccountsInstruction(
			&timelock_token_v1.CloseAccountsInstructionAccounts{
				Timelock:       a.State.publicKey.ToBytes(),
				Vault:          a.Vault.publicKey.ToBytes(),
				CloseAuthority: a.CloseAuthority.publicKey.ToBytes(),
				Payer:          GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_v1.CloseAccountsInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	case timelock_token_v1.DataVersionLegacy:
		return timelock_token_legacy.NewCloseAccountsInstruction(
			&timelock_token_legacy.CloseAccountsInstructionAccounts{
				Timelock:       a.State.publicKey.ToBytes(),
				Vault:          a.Vault.publicKey.ToBytes(),
				CloseAuthority: a.CloseAuthority.publicKey.ToBytes(),
				Payer:          GetSubsidizer().publicKey.ToBytes(),
			},
			&timelock_token_legacy.CloseAccountsInstructionArgs{
				TimelockBump: a.StateBump,
			},
		).ToLegacyInstruction(), nil
	default:
		return solana.Instruction{}, errors.New("unsupported data version")
	}
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
