package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

func RunTests(t *testing.T, s swap.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s swap.Store){
		testRoundTrip,
		testUpdateHappyPath,
		testUpdateStaleRecord,
		testGetAllByOwnerAndState,
		testGetAllByState,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s swap.Store) {
	t.Run("testRoundTrip", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.GetById(ctx, "test_swap_id")
		require.Error(t, err)
		assert.Equal(t, swap.ErrNotFound, err)
		assert.Nil(t, actual)

		expected := &swap.Record{
			SwapId: "test_swap_id",

			Owner: "test_owner",

			FromMint: "test_from_mint",
			ToMint:   "test_to_mint",
			Amount:   12345,

			FundingId:     "test_funding_id",
			FundingSource: swap.FundingSourceSubmitIntent,

			Nonce:     "test_nonce",
			Blockhash: "test_blockhash",

			ProofSignature: "test_proof_signature",

			TransactionSignature: pointer.String("test_transaction_signature"),
			TransactionBlob:      []byte("test_transaction_blob"),

			State: swap.StateConfirmed,

			CreatedAt: time.Now(),
		}
		cloned := expected.Clone()
		err = s.Save(ctx, expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		actual, err = s.GetById(ctx, "test_swap_id")
		require.NoError(t, err)
		assertEquivalentRecords(t, &cloned, actual)
	})
}

func testUpdateHappyPath(t *testing.T, s swap.Store) {
	t.Run("testUpdateHappyPath", func(t *testing.T) {
		ctx := context.Background()

		actual, err := s.GetById(ctx, "test_swap_id")
		require.Error(t, err)
		assert.Equal(t, swap.ErrNotFound, err)
		assert.Nil(t, actual)

		expected := &swap.Record{
			SwapId: "test_swap_id",

			Owner: "test_owner",

			FromMint: "test_from_mint",
			ToMint:   "test_to_mint",
			Amount:   12345,

			FundingId:     "test_funding_id",
			FundingSource: swap.FundingSourceSubmitIntent,

			Nonce:     "test_nonce",
			Blockhash: "test_blockhash",

			ProofSignature: "test_proof_signature",

			TransactionSignature: nil,
			TransactionBlob:      nil,

			State: swap.StateUnknown,

			CreatedAt: time.Now(),
		}
		err = s.Save(ctx, expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		expected.TransactionSignature = pointer.String("test_transaction_signature")
		expected.TransactionBlob = []byte("transaction_blob")
		expected.State = swap.StateConfirmed

		err = s.Save(ctx, expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 2, expected.Version)

		actual, err = s.GetById(ctx, "test_swap_id")
		require.NoError(t, err)
		assertEquivalentRecords(t, expected, actual)
	})
}

func testUpdateStaleRecord(t *testing.T, s swap.Store) {
	t.Run("testUpdateStaleRecord", func(t *testing.T) {
		ctx := context.Background()

		expected := &swap.Record{
			SwapId: "test_swap_id",

			Owner: "test_owner",

			FromMint: "test_from_mint",
			ToMint:   "test_to_mint",
			Amount:   12345,

			FundingId:     "test_funding_id",
			FundingSource: swap.FundingSourceSubmitIntent,

			Nonce:     "test_nonce",
			Blockhash: "test_blockhash",

			ProofSignature: "test_proof_signature",

			TransactionSignature: pointer.String("test_transaction_signature"),
			TransactionBlob:      []byte("test_transaction_blob"),

			State: swap.StateConfirmed,

			CreatedAt: time.Now(),
		}
		err := s.Save(ctx, expected)
		require.NoError(t, err)
		assert.EqualValues(t, 1, expected.Id)
		assert.EqualValues(t, 1, expected.Version)

		stale := expected.Clone()
		expected.State = swap.StateUnknown
		expected.TransactionSignature = nil
		expected.TransactionBlob = nil
		stale.Version -= 1

		err = s.Save(ctx, &stale)
		assert.Equal(t, swap.ErrStaleVersion, err)
		assert.EqualValues(t, 1, stale.Id)
		assert.EqualValues(t, 0, stale.Version)

		actual, err := s.GetById(ctx, "test_swap_id")
		require.NoError(t, err)
		assert.Equal(t, swap.StateConfirmed, actual.State)
		assert.NotNil(t, actual.TransactionSignature)
		assert.NotEmpty(t, actual.TransactionBlob)
		assert.EqualValues(t, 1, actual.Id)
		assert.EqualValues(t, 1, actual.Version)
	})
}

func testGetAllByOwnerAndState(t *testing.T, s swap.Store) {
	t.Run("testGetAllByOwnerAndState", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllByOwnerAndState(ctx, "test_owner_0", swap.StateConfirmed)
		assert.Equal(t, swap.ErrNotFound, err)

		var records []*swap.Record
		for i := range 100 {
			record := &swap.Record{
				SwapId: fmt.Sprintf("test_swap_id_%d", i),

				Owner: fmt.Sprintf("test_owner_%d", i%3),

				FromMint: fmt.Sprintf("test_from_mint_%d", i),
				ToMint:   fmt.Sprintf("test_to_mint_%d", i),
				Amount:   uint64(i + 1),

				FundingId:     fmt.Sprintf("test_funding_id_%d", i),
				FundingSource: swap.FundingSourceSubmitIntent,

				Nonce:     fmt.Sprintf("test_nonce_%d", i),
				Blockhash: fmt.Sprintf("test_blockhash_%d", i),

				ProofSignature: fmt.Sprintf("test_proof_signature_%d", i),

				TransactionSignature: pointer.String(fmt.Sprintf("test_transaction_signature_%d", i)),
				TransactionBlob:      []byte(fmt.Sprintf("test_transaction_blob_%d", i)),

				State: swap.State(i % int(swap.StateConfirmed+1)),

				CreatedAt: time.Now(),
			}
			require.NoError(t, s.Save(ctx, record))

			records = append(records, record)
		}

		allActual, err := s.GetAllByOwnerAndState(ctx, "test_owner_0", swap.StateConfirmed)
		require.NoError(t, err)
		require.NotEmpty(t, allActual)

		for _, record := range records {
			if record.Owner == "test_owner_0" && record.State == swap.StateConfirmed {
				var found bool
				for _, actual := range allActual {
					if actual.SwapId == record.SwapId {
						found = true
						assertEquivalentRecords(t, record, actual)
						break
					}
				}
				assert.True(t, found)
			}
		}
	})
}

func testGetAllByState(t *testing.T, s swap.Store) {
	t.Run("testGetAllByState", func(t *testing.T) {
		ctx := context.Background()

		_, err := s.GetAllByState(ctx, swap.StateConfirmed, query.EmptyCursor, 1, query.Ascending)
		assert.Equal(t, swap.ErrNotFound, err)

		var records []*swap.Record
		for i := range 100 {
			state := swap.StateConfirmed
			if i >= 50 {
				state = swap.StateUnknown
			}

			record := &swap.Record{
				SwapId: fmt.Sprintf("test_swap_id_%d", i),

				Owner: fmt.Sprintf("test_owner_%d", i%3),

				FromMint: "test_from_mint",
				ToMint:   "test_to_mint",
				Amount:   uint64(i + 1),

				FundingId:     fmt.Sprintf("test_funding_id_%d", i),
				FundingSource: swap.FundingSourceSubmitIntent,

				Nonce:     fmt.Sprintf("test_nonce_%d", i),
				Blockhash: fmt.Sprintf("test_blockhash_%d", i),

				ProofSignature: fmt.Sprintf("test_proof_signature_%d", i),

				TransactionSignature: pointer.String(fmt.Sprintf("test_transaction_signature_%d", i)),
				TransactionBlob:      []byte(fmt.Sprintf("test_transaction_blob_%d", i)),

				State: state,

				CreatedAt: time.Now(),
			}
			require.NoError(t, s.Save(ctx, record))

			records = append(records, record)
		}

		allActual, err := s.GetAllByState(ctx, swap.StateConfirmed, query.EmptyCursor, 100, query.Ascending)
		require.NoError(t, err)
		require.Len(t, allActual, 50)
		for i, actual := range allActual {
			assertEquivalentRecords(t, records[i], actual)
		}

		allActual, err = s.GetAllByState(ctx, swap.StateConfirmed, query.EmptyCursor, 10, query.Ascending)
		require.NoError(t, err)
		require.Len(t, allActual, 10)
		for i, actual := range allActual {
			assertEquivalentRecords(t, records[i], actual)
		}

		allActual, err = s.GetAllByState(ctx, swap.StateConfirmed, query.EmptyCursor, 10, query.Descending)
		require.NoError(t, err)
		require.Len(t, allActual, 10)
		for i, actual := range allActual {
			assertEquivalentRecords(t, records[50-i-1], actual)
		}

		allActual, err = s.GetAllByState(ctx, swap.StateConfirmed, query.ToCursor(records[23].Id), 10, query.Ascending)
		require.NoError(t, err)
		require.Len(t, allActual, 10)
		for i, actual := range allActual {
			assertEquivalentRecords(t, records[23+i+1], actual)
		}

		allActual, err = s.GetAllByState(ctx, swap.StateConfirmed, query.ToCursor(records[23].Id), 10, query.Descending)
		require.NoError(t, err)
		require.Len(t, allActual, 10)
		for i, actual := range allActual {
			assertEquivalentRecords(t, records[23-i-1], actual)
		}

		_, err = s.GetAllByState(ctx, swap.StateConfirmed, query.ToCursor(records[50].Id), 10, query.Ascending)
		assert.Equal(t, swap.ErrNotFound, err)
	})
}

func assertEquivalentRecords(t *testing.T, obj1, obj2 *swap.Record) {
	assert.Equal(t, obj1.SwapId, obj2.SwapId)

	assert.Equal(t, obj1.Owner, obj2.Owner)

	assert.Equal(t, obj1.FromMint, obj2.FromMint)
	assert.Equal(t, obj1.ToMint, obj2.ToMint)
	assert.Equal(t, obj1.Amount, obj2.Amount)

	assert.Equal(t, obj1.FundingId, obj2.FundingId)
	assert.Equal(t, obj1.FundingSource, obj2.FundingSource)

	assert.Equal(t, obj1.Nonce, obj2.Nonce)
	assert.Equal(t, obj1.Blockhash, obj2.Blockhash)

	assert.Equal(t, obj1.ProofSignature, obj2.ProofSignature)

	assert.EqualValues(t, obj1.TransactionSignature, obj2.TransactionSignature)
	assert.Equal(t, obj1.TransactionBlob, obj2.TransactionBlob)

	assert.Equal(t, obj1.State, obj2.State)
}
