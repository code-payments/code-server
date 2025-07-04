package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

func TestGetOwnerMetadata_User12Words(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	vmAccount := newRandomTestAccount(t)
	subsidizerAccount = newRandomTestAccount(t)
	owner := newRandomTestAccount(t)
	swapAuthority := newRandomTestAccount(t)
	coreMintAccount := newRandomTestAccount(t)
	swapMintAccount := newRandomTestAccount(t)

	_, err := GetOwnerMetadata(ctx, data, owner)
	assert.Equal(t, ErrOwnerNotFound, err)

	// Later calls intent to OpenAccounts

	timelockAccounts, err := owner.GetTimelockAccounts(vmAccount, coreMintAccount)
	require.NoError(t, err)

	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	primaryAccountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: timelockRecord.VaultOwner,
		TokenAccount:     timelockRecord.VaultAddress,
		MintAccount:      coreMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_PRIMARY,
	}
	require.NoError(t, data.CreateAccountInfo(ctx, primaryAccountInfoRecord))

	actual, err := GetOwnerMetadata(ctx, data, owner)
	require.NoError(t, err)
	assert.Equal(t, actual.Account.PublicKey().ToBase58(), owner.PublicKey().ToBase58())
	assert.Equal(t, OwnerTypeUser12Words, actual.Type)
	assert.Equal(t, OwnerManagementStateCodeAccount, actual.State)

	// Add swap account

	swapAta, err := swapAuthority.ToAssociatedTokenAccount(swapMintAccount)
	require.NoError(t, err)
	swapAccountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: swapAuthority.PublicKey().ToBase58(),
		TokenAccount:     swapAta.PublicKey().ToBase58(),
		MintAccount:      swapMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_SWAP,
	}
	require.NoError(t, data.CreateAccountInfo(ctx, swapAccountInfoRecord))

	actual, err = GetOwnerMetadata(ctx, data, owner)
	require.NoError(t, err)
	assert.Equal(t, actual.Account.PublicKey().ToBase58(), owner.PublicKey().ToBase58())
	assert.Equal(t, OwnerTypeUser12Words, actual.Type)
	assert.Equal(t, OwnerManagementStateCodeAccount, actual.State)

	// Unlock a Timelock account

	timelockRecord.VaultState = timelock_token_v1.StateWaitingForTimeout
	timelockRecord.Block += 1
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	actual, err = GetOwnerMetadata(ctx, data, owner)
	require.NoError(t, err)
	assert.Equal(t, actual.Account.PublicKey().ToBase58(), owner.PublicKey().ToBase58())
	assert.Equal(t, OwnerTypeUser12Words, actual.Type)
	assert.Equal(t, OwnerManagementStateUnlocked, actual.State)
}

func TestGetOwnerMetadata_RemoteSendGiftCard(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	vmAccount := newRandomTestAccount(t)
	subsidizerAccount = newRandomTestAccount(t)
	owner := newRandomTestAccount(t)
	mintAccount := newRandomTestAccount(t)

	_, err := GetOwnerMetadata(ctx, data, owner)
	assert.Equal(t, ErrOwnerNotFound, err)

	timelockAccounts, err := owner.GetTimelockAccounts(vmAccount, mintAccount)
	require.NoError(t, err)

	timelockRecord := timelockAccounts.ToDBRecord()
	require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: timelockRecord.VaultOwner,
		TokenAccount:     timelockRecord.VaultAddress,
		MintAccount:      mintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
	}
	require.NoError(t, data.CreateAccountInfo(ctx, accountInfoRecord))

	actual, err := GetOwnerMetadata(ctx, data, owner)
	require.NoError(t, err)
	assert.Equal(t, actual.Account.PublicKey().ToBase58(), owner.PublicKey().ToBase58())
	assert.Equal(t, OwnerTypeRemoteSendGiftCard, actual.Type)
	assert.Equal(t, OwnerManagementStateCodeAccount, actual.State)
}

func TestGetLatestTokenAccountRecordsForOwner(t *testing.T) {
	ctx := context.Background()
	data := code_data.NewTestDataProvider()

	vmAccount := newRandomTestAccount(t)
	subsidizerAccount = newRandomTestAccount(t)
	owner := newRandomTestAccount(t)
	coreMintAccount := newRandomTestAccount(t)
	swapMintAccount := newRandomTestAccount(t)

	actual, err := GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	require.NoError(t, err)
	assert.Empty(t, actual)

	authority1 := newRandomTestAccount(t)
	authority2 := newRandomTestAccount(t)
	authority3 := newRandomTestAccount(t)
	authority4 := newRandomTestAccount(t)
	authority5 := newRandomTestAccount(t)
	authority6 := newRandomTestAccount(t)
	authority7 := newRandomTestAccount(t)

	for _, authorityAndType := range []struct {
		account     *Account
		accountType commonpb.AccountType
	}{
		{authority1, commonpb.AccountType_BUCKET_1_KIN},
		{authority2, commonpb.AccountType_BUCKET_10_KIN},
	} {
		timelockAccounts, err := authorityAndType.account.GetTimelockAccounts(vmAccount, coreMintAccount)
		require.NoError(t, err)

		timelockRecord := timelockAccounts.ToDBRecord()
		require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

		accountInfoRecord := &account.Record{
			OwnerAccount:     owner.PublicKey().ToBase58(),
			AuthorityAccount: timelockRecord.VaultOwner,
			TokenAccount:     timelockRecord.VaultAddress,
			MintAccount:      coreMintAccount.PublicKey().ToBase58(),
			AccountType:      authorityAndType.accountType,
		}
		require.NoError(t, data.CreateAccountInfo(ctx, accountInfoRecord))
	}

	for _, authorityAndRelationship := range []struct {
		account *Account
		domain  string
	}{
		{authority3, "app1.com"},
		{authority4, "app2.com"},
	} {
		timelockAccounts, err := authorityAndRelationship.account.GetTimelockAccounts(vmAccount, coreMintAccount)
		require.NoError(t, err)

		timelockRecord := timelockAccounts.ToDBRecord()
		require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

		accountInfoRecord := &account.Record{
			OwnerAccount:     owner.PublicKey().ToBase58(),
			AuthorityAccount: timelockRecord.VaultOwner,
			TokenAccount:     timelockRecord.VaultAddress,
			MintAccount:      coreMintAccount.PublicKey().ToBase58(),
			AccountType:      commonpb.AccountType_RELATIONSHIP,
			RelationshipTo:   &authorityAndRelationship.domain,
		}
		require.NoError(t, data.CreateAccountInfo(ctx, accountInfoRecord))
	}

	swapAta, err := owner.ToAssociatedTokenAccount(authority5)
	require.NoError(t, err)
	swapAccountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: authority5.PublicKey().ToBase58(),
		TokenAccount:     swapAta.PublicKey().ToBase58(),
		MintAccount:      swapMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_SWAP,
	}
	require.NoError(t, data.CreateAccountInfo(ctx, swapAccountInfoRecord))

	for i, authority := range []*Account{
		authority6,
		authority7,
	} {
		timelockAccounts, err := authority.GetTimelockAccounts(vmAccount, coreMintAccount)
		require.NoError(t, err)

		timelockRecord := timelockAccounts.ToDBRecord()
		require.NoError(t, data.SaveTimelock(ctx, timelockRecord))

		accountInfoRecord := &account.Record{
			OwnerAccount:     owner.PublicKey().ToBase58(),
			AuthorityAccount: timelockRecord.VaultOwner,
			TokenAccount:     timelockRecord.VaultAddress,
			MintAccount:      coreMintAccount.PublicKey().ToBase58(),
			AccountType:      commonpb.AccountType_POOL,
			Index:            uint64(i),
		}
		require.NoError(t, data.CreateAccountInfo(ctx, accountInfoRecord))
	}

	actual, err = GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	require.NoError(t, err)
	require.Len(t, actual, 5)

	records, ok := actual[commonpb.AccountType_BUCKET_1_KIN]
	require.True(t, ok)
	require.Len(t, records, 1)
	assert.Equal(t, records[0].General.AuthorityAccount, authority1.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.AccountType, commonpb.AccountType_BUCKET_1_KIN)
	assert.Equal(t, records[0].Timelock.VaultOwner, authority1.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.TokenAccount, records[0].Timelock.VaultAddress)

	records, ok = actual[commonpb.AccountType_BUCKET_10_KIN]
	require.True(t, ok)
	require.Len(t, records, 1)
	assert.Equal(t, records[0].General.AuthorityAccount, authority2.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.AccountType, commonpb.AccountType_BUCKET_10_KIN)
	assert.Equal(t, records[0].Timelock.VaultOwner, authority2.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.TokenAccount, records[0].Timelock.VaultAddress)

	records, ok = actual[commonpb.AccountType_RELATIONSHIP]
	require.True(t, ok)
	require.Len(t, records, 2)

	assert.Equal(t, records[0].General.AuthorityAccount, authority3.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.AccountType, commonpb.AccountType_RELATIONSHIP)
	assert.Equal(t, records[0].Timelock.VaultOwner, authority3.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.TokenAccount, records[0].Timelock.VaultAddress)
	assert.Equal(t, *records[0].General.RelationshipTo, "app1.com")

	assert.Equal(t, records[1].General.AuthorityAccount, authority4.PublicKey().ToBase58())
	assert.Equal(t, records[1].General.AccountType, commonpb.AccountType_RELATIONSHIP)
	assert.Equal(t, records[1].Timelock.VaultOwner, authority4.PublicKey().ToBase58())
	assert.Equal(t, records[1].General.TokenAccount, records[1].Timelock.VaultAddress)
	assert.Equal(t, *records[1].General.RelationshipTo, "app2.com")

	records, ok = actual[commonpb.AccountType_SWAP]
	require.True(t, ok)
	require.Len(t, records, 1)
	assert.Nil(t, records[0].Timelock)
	assert.Equal(t, records[0].General.AuthorityAccount, authority5.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.TokenAccount, swapAta.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.AccountType, commonpb.AccountType_SWAP)

	records, ok = actual[commonpb.AccountType_POOL]
	require.True(t, ok)
	require.Len(t, records, 2)

	assert.Equal(t, records[0].General.AuthorityAccount, authority6.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.AccountType, commonpb.AccountType_POOL)
	assert.Equal(t, records[0].Timelock.VaultOwner, authority6.PublicKey().ToBase58())
	assert.Equal(t, records[0].General.TokenAccount, records[0].Timelock.VaultAddress)
	assert.EqualValues(t, records[0].General.Index, 0)

	assert.Equal(t, records[1].General.AuthorityAccount, authority7.PublicKey().ToBase58())
	assert.Equal(t, records[1].General.AccountType, commonpb.AccountType_POOL)
	assert.Equal(t, records[1].Timelock.VaultOwner, authority7.PublicKey().ToBase58())
	assert.Equal(t, records[1].General.TokenAccount, records[1].Timelock.VaultAddress)
	assert.EqualValues(t, records[1].General.Index, 1)
}
