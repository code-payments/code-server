package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/data/account"
)

func RunTests(t *testing.T, s account.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s account.Store){
		testRoundTrip,
		testPutMultipleRecords,
		testPutErrors,
		testGetLatestByOwner,
		testBatchedMethods,
		testRemoteSendEdgeCases,
		testSwapAccountEdgeCases,
		testDepositSyncMethods,
		testAutoReturnCheckMethods,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s account.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		start := time.Now()

		expected := &account.Record{
			OwnerAccount:         "owner",
			AuthorityAccount:     "authority",
			TokenAccount:         "token",
			MintAccount:          "mint",
			AccountType:          commonpb.AccountType_POOL,
			Index:                123,
			RequiresDepositSync:  true,
			DepositsLastSyncedAt: time.Now().Add(-time.Hour),
		}
		cloned := expected.Clone()

		_, err := s.GetByTokenAddress(ctx, cloned.TokenAccount)
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		err = s.Update(ctx, expected)
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		require.NoError(t, s.Put(ctx, expected))

		assert.True(t, expected.Id > 0)
		assert.True(t, expected.CreatedAt.After(start))

		actual, err := s.GetByTokenAddress(ctx, cloned.TokenAccount)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err := s.GetByAuthorityAddress(ctx, cloned.AuthorityAccount)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assertEquivalentRecords(t, &cloned, actual)

		expected.RequiresDepositSync = false
		expected.DepositsLastSyncedAt = time.Now()
		cloned = expected.Clone()
		require.NoError(t, s.Update(ctx, expected))

		actual, err = s.GetByTokenAddress(ctx, cloned.TokenAccount)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err = s.GetByAuthorityAddress(ctx, cloned.AuthorityAccount)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testPutMultipleRecords(t *testing.T, s account.Store) {
	t.Run("testPutMultipleRecords", func(t *testing.T) {
		ctx := context.Background()

		var records []*account.Record

		// Accounts within the same type case
		for i := 0; i < 5; i++ {
			record := &account.Record{
				OwnerAccount:     "owner_part1",
				AuthorityAccount: fmt.Sprintf("authority_part1_%d", i),
				TokenAccount:     fmt.Sprintf("token_part1_%d", i),
				MintAccount:      "mint",
				AccountType:      commonpb.AccountType_POOL,
				Index:            uint64(i),
			}
			cloned := record.Clone()

			require.NoError(t, s.Put(ctx, record))

			records = append(records, &cloned)
		}

		// Accounts across different type case
		for i, accountType := range []commonpb.AccountType{
			commonpb.AccountType_PRIMARY,
			commonpb.AccountType_SWAP,
		} {
			record := &account.Record{
				OwnerAccount:     "owner_part2",
				AuthorityAccount: fmt.Sprintf("authority_part2_%d", i),
				TokenAccount:     fmt.Sprintf("token_part2_%d", i),
				MintAccount:      "mint",
				AccountType:      accountType,
				Index:            0,
			}
			if accountType == commonpb.AccountType_PRIMARY {
				record.AuthorityAccount = record.OwnerAccount
			}
			cloned := record.Clone()

			require.NoError(t, s.Put(ctx, record))

			records = append(records, &cloned)
		}

		// Accounts across different mints
		for i := 0; i < 5; i++ {
			record := &account.Record{
				OwnerAccount:     "owner_part3",
				AuthorityAccount: "owner_part3",
				TokenAccount:     fmt.Sprintf("token_part3_%d", i),
				MintAccount:      fmt.Sprintf("mint%d", i),
				AccountType:      commonpb.AccountType_PRIMARY,
				Index:            0,
			}
			cloned := record.Clone()

			require.NoError(t, s.Put(ctx, record))

			records = append(records, &cloned)
		}

		for _, expected := range records {
			actual, err := s.GetByTokenAddress(ctx, expected.TokenAccount)
			require.NoError(t, err)
			assertEquivalentRecords(t, expected, actual)
		}
	})
}

func testPutErrors(t *testing.T, s account.Store) {
	t.Run("testPutErrors", func(t *testing.T) {
		ctx := context.Background()

		record := &account.Record{
			OwnerAccount:     "owner",
			AuthorityAccount: "authority",
			TokenAccount:     "token",
			MintAccount:      "mint",
			AccountType:      commonpb.AccountType_POOL,
			Index:            0,
		}
		original := record.Clone()

		require.NoError(t, s.Put(ctx, record))

		assert.Equal(t, account.ErrAccountInfoExists, s.Put(ctx, record))

		// Cannot change any 1 field

		cloned := original.Clone()
		cloned.OwnerAccount = "new_owner"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.AuthorityAccount = "new_authority"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.TokenAccount = "new_token"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.AccountType = commonpb.AccountType_SWAP
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		// Changing multiple fields with owner changed

		cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.AuthorityAccount = "new_authority"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.AuthorityAccount = "new_authority"
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.TokenAccount = "new_token"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.TokenAccount = "new_token"
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.TokenAccount = "new_token"
		cloned.AccountType = commonpb.AccountType_SWAP
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		// todo: this case isn't possible with current account structures
		/*cloned = original.Clone()
		cloned.OwnerAccount = "new_owner"
		cloned.TokenAccount = "new_token"
		cloned.AccountType = ?
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))*/

		// Changing multiple fields with token changed

		cloned = original.Clone()
		cloned.TokenAccount = "new_token"
		cloned.AuthorityAccount = "new_authority"
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		cloned = original.Clone()
		cloned.TokenAccount = "new_token"
		cloned.AccountType = commonpb.AccountType_SWAP
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		// todo: this case isn't possible with current account structures
		/*cloned = original.Clone()
		cloned.TokenAccount = "new_token"
		cloned.AccountType = ?
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))*/

		// Changing multiple fields with authority changed

		cloned = original.Clone()
		cloned.AuthorityAccount = "new_authority"
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))

		// todo: this case isn't possible with current account structures
		/*cloned = original.Clone()
		cloned.AuthorityAccount = "new_authority"
		cloned.AccountType = ?
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))*/

		// Changing multiple fields with account type changed

		// todo: this case isn't possible with current account structures
		/*cloned = original.Clone()
		cloned.AccountType = ?
		cloned.Index = cloned.Index + 1
		assert.Equal(t, account.ErrInvalidAccountInfo, s.Put(ctx, &cloned))*/

		// Ensure we didn't overwrite the original record
		actual, err := s.GetByTokenAddress(ctx, original.TokenAccount)
		require.NoError(t, err)
		assertEquivalentRecords(t, &original, actual)
	})
}

func testGetLatestByOwner(t *testing.T, s account.Store) {
	t.Run("testGetLatestByOwner", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetLatestByOwnerAddress(ctx, "owner")
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		_, err = s.GetLatestByOwnerAddressAndType(ctx, "owner", commonpb.AccountType_POOL)
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		for _, mint := range []string{"mint1", "mint2"} {
			for _, accountType := range []commonpb.AccountType{
				commonpb.AccountType_PRIMARY,
				commonpb.AccountType_SWAP,
			} {
				record := &account.Record{
					OwnerAccount:     "owner",
					AuthorityAccount: fmt.Sprintf("authority_%s", accountType.String()),
					TokenAccount:     fmt.Sprintf("token_%s_%s", accountType.String(), mint),
					MintAccount:      mint,
					AccountType:      accountType,
					Index:            0,
				}
				if accountType == commonpb.AccountType_PRIMARY {
					record.AuthorityAccount = record.OwnerAccount
				}

				require.NoError(t, s.Put(ctx, record))
			}
		}

		for _, mint := range []string{"mint1", "mint2"} {
			for i, accountType := range []commonpb.AccountType{
				commonpb.AccountType_POOL,
			} {
				for j := 0; j < 5; j++ {
					record := &account.Record{
						OwnerAccount:     "owner",
						AuthorityAccount: fmt.Sprintf("authority_%s_%d%d", accountType.String(), i, j),
						TokenAccount:     fmt.Sprintf("token_%s_%s_%d%d", accountType.String(), mint, i, j),
						MintAccount:      mint,
						AccountType:      accountType,
						Index:            uint64(j),
					}
					require.NoError(t, s.Put(ctx, record))
				}
			}
		}

		actualByMintAndType, err := s.GetLatestByOwnerAddress(ctx, "owner")
		require.NoError(t, err)
		require.Len(t, actualByMintAndType, 2)
		for _, mint := range []string{"mint1", "mint2"} {
			actualByType, ok := actualByMintAndType[mint]
			require.True(t, ok)

			allActual, ok := actualByType[commonpb.AccountType_PRIMARY]
			require.True(t, ok)
			require.Len(t, allActual, 1)
			assert.Equal(t, fmt.Sprintf("token_PRIMARY_%s", mint), allActual[0].TokenAccount)

			allActual, ok = actualByType[commonpb.AccountType_SWAP]
			require.True(t, ok)
			require.Len(t, allActual, 1)
			assert.Equal(t, fmt.Sprintf("token_SWAP_%s", mint), allActual[0].TokenAccount)

			allActual, ok = actualByType[commonpb.AccountType_POOL]
			require.True(t, ok)
			require.Len(t, allActual, 5)
			for i, actual := range allActual {
				assert.Equal(t, fmt.Sprintf("token_POOL_%s_0%d", mint, i), actual.TokenAccount)
			}
		}

		actualByMint, err := s.GetLatestByOwnerAddressAndType(ctx, "owner", commonpb.AccountType_POOL)
		require.NoError(t, err)
		require.Len(t, actualByMint, 2)
		for _, mint := range []string{"mint1", "mint2"} {
			actual, ok := actualByMint[mint]
			require.True(t, ok)
			assert.Equal(t, fmt.Sprintf("token_POOL_%s_04", mint), actual.TokenAccount)
		}
	})
}

func testBatchedMethods(t *testing.T, s account.Store) {
	t.Run("testBatchedMethods", func(t *testing.T) {
		ctx := context.Background()

		var records []*account.Record
		for i := 0; i < 100; i++ {
			record := &account.Record{
				OwnerAccount:     fmt.Sprintf("owner%d", i),
				AuthorityAccount: fmt.Sprintf("authority%d", i),
				TokenAccount:     fmt.Sprintf("token%d", i),
				MintAccount:      fmt.Sprintf("mint%d", i),
				AccountType:      commonpb.AccountType_POOL,
				Index:            uint64(i),
			}

			require.NoError(t, s.Put(ctx, record))

			records = append(records, record)
		}

		actual, err := s.GetByTokenAddressBatch(ctx, "token0", "token1")
		require.NoError(t, err)
		require.Len(t, actual, 2)
		assertEquivalentRecords(t, records[0], actual[records[0].TokenAccount])
		assertEquivalentRecords(t, records[1], actual[records[1].TokenAccount])

		actual, err = s.GetByTokenAddressBatch(ctx, "token0", "token1", "token2", "token3", "token4")
		require.NoError(t, err)
		require.Len(t, actual, 5)
		assertEquivalentRecords(t, records[0], actual[records[0].TokenAccount])
		assertEquivalentRecords(t, records[1], actual[records[1].TokenAccount])
		assertEquivalentRecords(t, records[2], actual[records[2].TokenAccount])
		assertEquivalentRecords(t, records[3], actual[records[3].TokenAccount])
		assertEquivalentRecords(t, records[4], actual[records[4].TokenAccount])

		_, err = s.GetByTokenAddressBatch(ctx, "not-found")
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		_, err = s.GetByTokenAddressBatch(ctx, "token0", "not-found")
		assert.Equal(t, account.ErrAccountInfoNotFound, err)
	})
}

func testRemoteSendEdgeCases(t *testing.T, s account.Store) {
	t.Run("testRemoteSendEdgeCases", func(t *testing.T) {
		ctx := context.Background()

		remoteSendRecord := &account.Record{
			OwnerAccount:            "owner",
			AuthorityAccount:        "owner",
			TokenAccount:            "token",
			MintAccount:             "mint",
			AccountType:             commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
			Index:                   uint64(0),
			RequiresAutoReturnCheck: true,
		}
		cloned := remoteSendRecord.Clone()

		primaryRecord := remoteSendRecord.Clone()
		primaryRecord.AccountType = commonpb.AccountType_PRIMARY

		require.NoError(t, s.Put(ctx, remoteSendRecord))
		assert.Error(t, s.Put(ctx, &primaryRecord))

		actual, err := s.GetByTokenAddress(ctx, "token")
		require.NoError(t, err)
		assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, actual.AccountType)
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err := s.GetByAuthorityAddress(ctx, cloned.AuthorityAccount)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, actual.AccountType)
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err = s.GetLatestByOwnerAddressAndType(ctx, "owner", commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, actual.AccountType)
		assertEquivalentRecords(t, &cloned, actual)

		latestByMintAndType, err := s.GetLatestByOwnerAddress(ctx, "owner")
		require.NoError(t, err)
		require.Len(t, latestByMintAndType, 1)
		require.Len(t, latestByMintAndType[cloned.MintAccount], 1)
		records, ok := latestByMintAndType[cloned.MintAccount][commonpb.AccountType_REMOTE_SEND_GIFT_CARD]
		require.True(t, ok)
		require.Len(t, records, 1)
		actual = records[0]
		assert.Equal(t, commonpb.AccountType_REMOTE_SEND_GIFT_CARD, actual.AccountType)
		assertEquivalentRecords(t, &cloned, actual)

		remoteSendRecord.RequiresAutoReturnCheck = false
		cloned = remoteSendRecord.Clone()

		require.NoError(t, s.Update(ctx, remoteSendRecord))

		actual, err = s.GetByTokenAddress(ctx, "token")
		require.NoError(t, err)
		assert.False(t, actual.RequiresAutoReturnCheck)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testSwapAccountEdgeCases(t *testing.T, s account.Store) {
	t.Run("testSwapAccountEdgeCases", func(t *testing.T) {
		ctx := context.Background()

		swapRecord := &account.Record{
			OwnerAccount:     "owner",
			AuthorityAccount: "authority",
			TokenAccount:     "token1",
			MintAccount:      "mint1",
			AccountType:      commonpb.AccountType_SWAP,
			Index:            uint64(0),
		}
		cloned := swapRecord.Clone()

		require.NoError(t, s.Put(ctx, swapRecord))
		assert.Equal(t, account.ErrAccountInfoExists, s.Put(ctx, swapRecord))

		actual, err := s.GetByTokenAddress(ctx, cloned.TokenAccount)
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err := s.GetByAuthorityAddress(ctx, cloned.AuthorityAccount)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assertEquivalentRecords(t, &cloned, actual)

		actualByMint, err = s.GetLatestByOwnerAddressAndType(ctx, cloned.OwnerAccount, commonpb.AccountType_SWAP)
		require.NoError(t, err)
		require.Len(t, actualByMint, 1)
		actual = actualByMint[cloned.MintAccount]
		assertEquivalentRecords(t, &cloned, actual)

		actualByMintAndType, err := s.GetLatestByOwnerAddress(ctx, cloned.OwnerAccount)
		require.NoError(t, err)
		require.Len(t, actualByMintAndType, 1)
		require.Len(t, actualByMintAndType[actual.MintAccount], 1)
		require.Len(t, actualByMintAndType[actual.MintAccount][commonpb.AccountType_SWAP], 1)
		actual = actualByMintAndType[actual.MintAccount][commonpb.AccountType_SWAP][0]
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testDepositSyncMethods(t *testing.T, s account.Store) {
	t.Run("testDepositSyncMethods", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetPrioritizedRequiringDepositSync(ctx, 10)
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		count, err := s.CountRequiringDepositSync(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		var records []*account.Record
		for i := 0; i < 10; i++ {
			record := &account.Record{
				OwnerAccount:         fmt.Sprintf("owner%d", i),
				AuthorityAccount:     fmt.Sprintf("owner%d", i),
				TokenAccount:         fmt.Sprintf("token%d", i),
				MintAccount:          "mint",
				AccountType:          commonpb.AccountType_PRIMARY,
				Index:                uint64(0),
				DepositsLastSyncedAt: time.Now().Add(time.Duration(-i) * time.Hour),
			}

			if i < 7 {
				record.RequiresDepositSync = true
			}

			require.NoError(t, s.Put(ctx, record))
			records = append(records, record)
		}

		count, err = s.CountRequiringDepositSync(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 7, count)

		result, err := s.GetPrioritizedRequiringDepositSync(ctx, 10)
		require.NoError(t, err)
		require.Len(t, result, 7)

		for i, actual := range result {
			assertEquivalentRecords(t, records[6-i], actual)
		}

		result, err = s.GetPrioritizedRequiringDepositSync(ctx, 3)
		require.NoError(t, err)
		require.Len(t, result, 3)

		for i, actual := range result {
			assertEquivalentRecords(t, records[6-i], actual)
		}
	})
}

func testAutoReturnCheckMethods(t *testing.T, s account.Store) {
	t.Run("testAutoReturnCheckMethods", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetPrioritizedRequiringAutoReturnCheck(ctx, time.Duration(0), 10)
		assert.Equal(t, account.ErrAccountInfoNotFound, err)

		count, err := s.CountRequiringAutoReturnCheck(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 0, count)

		var records []*account.Record
		for i := 0; i < 10; i++ {
			record := &account.Record{
				OwnerAccount:     fmt.Sprintf("owner%d", i),
				AuthorityAccount: fmt.Sprintf("owner%d", i),
				TokenAccount:     fmt.Sprintf("token%d", i),
				MintAccount:      "mint",
				AccountType:      commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
				Index:            uint64(0),
				CreatedAt:        time.Now().Add(time.Duration(-i) * time.Hour),
			}

			if i < 7 {
				record.RequiresAutoReturnCheck = true
			}

			require.NoError(t, s.Put(ctx, record))
			records = append(records, record)
		}

		count, err = s.CountRequiringAutoReturnCheck(ctx)
		require.NoError(t, err)
		assert.EqualValues(t, 7, count)

		result, err := s.GetPrioritizedRequiringAutoReturnCheck(ctx, time.Duration(0), 10)
		require.NoError(t, err)
		require.Len(t, result, 7)

		for i, actual := range result {
			assertEquivalentRecords(t, records[6-i], actual)
		}

		result, err = s.GetPrioritizedRequiringAutoReturnCheck(ctx, time.Duration(0), 3)
		require.NoError(t, err)
		require.Len(t, result, 3)

		for i, actual := range result {
			assertEquivalentRecords(t, records[6-i], actual)
		}

		result, err = s.GetPrioritizedRequiringAutoReturnCheck(ctx, 2*time.Hour+time.Second, 10)
		require.NoError(t, err)
		require.Len(t, result, 4)

		for i, actual := range result {
			assertEquivalentRecords(t, records[6-i], actual)
		}
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *account.Record) {
	assert.Equal(t, obj1.OwnerAccount, obj2.OwnerAccount)
	assert.Equal(t, obj1.AuthorityAccount, obj2.AuthorityAccount)
	assert.Equal(t, obj1.TokenAccount, obj2.TokenAccount)
	assert.Equal(t, obj1.MintAccount, obj2.MintAccount)
	assert.Equal(t, obj1.AccountType, obj2.AccountType)
	assert.Equal(t, obj1.Index, obj2.Index)
	assert.Equal(t, obj1.RequiresDepositSync, obj2.RequiresDepositSync)
	assert.Equal(t, obj1.DepositsLastSyncedAt.Unix(), obj2.DepositsLastSyncedAt.Unix())
	assert.Equal(t, obj1.RequiresAutoReturnCheck, obj2.RequiresAutoReturnCheck)
	assert.Equal(t, obj1.RequiresSwapRetry, obj2.RequiresSwapRetry)
	assert.Equal(t, obj1.LastSwapRetryAt.Unix(), obj2.LastSwapRetryAt.Unix())
}
