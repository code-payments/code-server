package common

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/mr-tron/base58/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/solana/token"
)

func TestAccountWithPublicKey(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	var accounts []*Account

	account, err := NewAccountFromPublicKeyBytes(publicKey)
	require.NoError(t, err)
	accounts = append(accounts, account)

	account, err = NewAccountFromPublicKeyString(base58.Encode(publicKey))
	require.NoError(t, err)
	accounts = append(accounts, account)

	account, err = NewAccountFromProto(&commonpb.SolanaAccountId{
		Value: publicKey,
	})
	require.NoError(t, err)
	accounts = append(accounts, account)

	for _, account := range accounts {
		assert.EqualValues(t, publicKey, account.PublicKey().ToBytes())
		assert.Nil(t, account.PrivateKey())

		protoValue := account.ToProto()
		require.NoError(t, protoValue.Validate())
		assert.EqualValues(t, publicKey, protoValue.Value)

		_, err = account.Sign([]byte("message"))
		assert.Error(t, err)
	}
}

func TestAccountWithPrivateKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	var accounts []*Account

	account, err := NewAccountFromPrivateKeyBytes(privateKey)
	require.NoError(t, err)
	accounts = append(accounts, account)

	account, err = NewAccountFromPrivateKeyString(base58.Encode(privateKey))
	require.NoError(t, err)
	accounts = append(accounts, account)

	for _, account := range accounts {
		assert.EqualValues(t, publicKey, account.PublicKey().ToBytes())
		assert.EqualValues(t, privateKey, account.PrivateKey().ToBytes())

		protoValue := account.ToProto()
		require.NoError(t, protoValue.Validate())
		assert.EqualValues(t, publicKey, protoValue.Value)

		message := []byte("message")
		signature, err := account.Sign(message)
		require.NoError(t, err)
		assert.Equal(t, ed25519.Sign(privateKey, message), signature)
	}
}

func TestInvalidAccount(t *testing.T) {
	stringValue := "invalid-account"
	bytesValue := []byte(stringValue)
	protoValue := &commonpb.SolanaAccountId{
		Value: bytesValue,
	}

	_, err := NewAccountFromPublicKeyBytes(bytesValue)
	assert.Error(t, err)

	_, err = NewAccountFromPublicKeyString(stringValue)
	assert.Error(t, err)

	_, err = NewAccountFromPrivateKeyBytes(bytesValue)
	assert.Error(t, err)

	_, err = NewAccountFromPrivateKeyString(stringValue)
	assert.Error(t, err)

	_, err = NewAccountFromProto(protoValue)
	assert.Error(t, err)
}

func TestConvertToTimelockVault(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	stateAddress, _, err := timelock_token_v1.GetStateAddress(&timelock_token_v1.GetStateAddressArgs{
		Mint:          mintAccount.PublicKey().ToBytes(),
		TimeAuthority: subsidizerAccount.PublicKey().ToBytes(),
		VaultOwner:    ownerAccount.PublicKey().ToBytes(),
		NumDaysLocked: timelock_token_v1.DefaultNumDaysLocked,
	})
	require.NoError(t, err)

	expectedVaultAddress, _, err := timelock_token_v1.GetVaultAddress(&timelock_token_v1.GetVaultAddressArgs{
		State:       stateAddress,
		DataVersion: timelock_token_v1.DataVersion1,
	})
	require.NoError(t, err)

	tokenAccount, err := ownerAccount.ToTimelockVault(mintAccount)
	require.NoError(t, err)
	assert.EqualValues(t, expectedVaultAddress, tokenAccount.PublicKey().ToBytes())
}

func TestGetTimelockAccounts(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	expectedStateAddress, expectedStateBump, err := timelock_token_v1.GetStateAddress(&timelock_token_v1.GetStateAddressArgs{
		Mint:          mintAccount.PublicKey().ToBytes(),
		TimeAuthority: subsidizerAccount.PublicKey().ToBytes(),
		VaultOwner:    ownerAccount.PublicKey().ToBytes(),
		NumDaysLocked: timelock_token_v1.DefaultNumDaysLocked,
	})
	require.NoError(t, err)

	expectedVaultAddress, expectedVaultBump, err := timelock_token_v1.GetVaultAddress(&timelock_token_v1.GetVaultAddressArgs{
		State:       expectedStateAddress,
		DataVersion: timelock_token_v1.DataVersion1,
	})
	require.NoError(t, err)

	actual, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)
	assert.EqualValues(t, expectedStateAddress, actual.State.PublicKey().ToBytes())
	assert.Equal(t, expectedStateBump, actual.StateBump)
	assert.EqualValues(t, expectedVaultAddress, actual.Vault.PublicKey().ToBytes())
	assert.Equal(t, expectedVaultBump, actual.VaultBump)
	assert.EqualValues(t, ownerAccount.PublicKey().ToBytes(), actual.VaultOwner.PublicKey().ToBytes())
	assert.EqualValues(t, mintAccount.PublicKey().ToBytes(), actual.Mint.PublicKey().ToBytes())
}

func TestIsAccountManagedByCode_TimelockState(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	// No record of the account anywhere
	result, err := timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.False(t, result)

	// The account is a locked timelock account with Code as the time and close authority
	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	result, err = timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.True(t, result)

	timelockRecord.VaultState = timelock_token_v1.StateLocked
	timelockRecord.Block += 1
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	result, err = timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.True(t, result)

	// The timelock account is waiting for timeout
	timelockRecord.VaultState = timelock_token_v1.StateWaitingForTimeout
	timelockRecord.Block += 1
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	result, err = timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.False(t, result)

	// The timelock account is unlocked
	timelockRecord.VaultState = timelock_token_v1.StateUnlocked
	timelockRecord.Block += 1
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	result, err = timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsAccountManagedByCode_OtherAccounts(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)
	require.NoError(t, data.SaveTimelock(ctx, timelockAccounts.ToDBRecord()))

	result, err := timelockAccounts.Vault.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.True(t, result)

	result, err = timelockAccounts.VaultOwner.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.False(t, result)

	result, err = timelockAccounts.State.IsManagedByCode(ctx, data)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestGetTransferWithAuthorityInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	source, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	destination := newRandomTestAccount(t)
	amount := kin.ToQuarks(123)

	ixn, err := source.GetTransferWithAuthorityInstruction(destination, amount)
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.TransferWithAuthorityInstructionFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, source.StateBump, args.TimelockBump)
	assert.Equal(t, amount, args.Amount)

	assert.EqualValues(t, source.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, source.Vault.PublicKey().ToBytes(), accounts.Vault)
	assert.EqualValues(t, ownerAccount.PublicKey().ToBytes(), accounts.VaultOwner)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.TimeAuthority)
	assert.EqualValues(t, destination.PublicKey().ToBytes(), accounts.Destination)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestGetWithdrawInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	source, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	destination := newRandomTestAccount(t)

	ixn, err := source.GetWithdrawInstruction(destination)
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.WithdrawInstructionFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, source.StateBump, args.TimelockBump)

	assert.EqualValues(t, source.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, source.Vault.PublicKey().ToBytes(), accounts.Vault)
	assert.EqualValues(t, ownerAccount.PublicKey().ToBytes(), accounts.VaultOwner)
	assert.EqualValues(t, destination.PublicKey().ToBytes(), accounts.Destination)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestGetBurnDustWithAuthorityInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	maxAmount := kin.ToQuarks(1)

	ixn, err := timelockAccounts.GetBurnDustWithAuthorityInstruction(maxAmount)
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.BurnDustWithAuthorityInstructionFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, timelockAccounts.StateBump, args.TimelockBump)
	assert.Equal(t, maxAmount, args.MaxAmount)

	assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), accounts.Vault)
	assert.EqualValues(t, ownerAccount.PublicKey().ToBytes(), accounts.VaultOwner)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.TimeAuthority)
	assert.EqualValues(t, mintAccount.PublicKey().ToBytes(), accounts.Mint)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestGetRevokeLockWithAuthorityInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	ixn, err := timelockAccounts.GetRevokeLockWithAuthorityInstruction()
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.RevokeLockWithAuthorityFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, timelockAccounts.StateBump, args.TimelockBump)

	assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), accounts.Vault)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.TimeAuthority)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestGetDeactivateInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	ixn, err := timelockAccounts.GetDeactivateInstruction()
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.DeactivateInstructionFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, timelockAccounts.StateBump, args.TimelockBump)

	assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, ownerAccount.PublicKey().ToBytes(), accounts.VaultOwner)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestGetCloseAccountsInstruction(t *testing.T) {
	subsidizerAccount = newRandomTestAccount(t)
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(mintAccount)
	require.NoError(t, err)

	ixn, err := timelockAccounts.GetCloseAccountsInstruction()
	require.NoError(t, err)

	txn := solana.NewTransaction(subsidizerAccount.PublicKey().ToBytes(), ixn)

	args, accounts, err := timelock_token_v1.CloseAccountsInstructionFromLegacyInstruction(txn, 0)
	require.NoError(t, err)

	assert.Equal(t, timelockAccounts.StateBump, args.TimelockBump)

	assert.EqualValues(t, timelockAccounts.State.PublicKey().ToBytes(), accounts.Timelock)
	assert.EqualValues(t, timelockAccounts.Vault.PublicKey().ToBytes(), accounts.Vault)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.CloseAuthority)
	assert.EqualValues(t, subsidizerAccount.PublicKey().ToBytes(), accounts.Payer)
}

func TestConvertToAssociatedTokenAccount(t *testing.T) {
	ownerAccount := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	expected, err := token.GetAssociatedAccount(ownerAccount.PublicKey().ToBytes(), mintAccount.PublicKey().ToBytes())
	require.NoError(t, err)

	actual, err := ownerAccount.ToAssociatedTokenAccount(mintAccount)
	require.NoError(t, err)

	assert.EqualValues(t, expected, actual.PublicKey().ToBytes())
}
