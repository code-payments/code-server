package tests

import (
	"context"
	"testing"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func RunTests(t *testing.T, s transaction.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s transaction.Store){
		testRoundTrip,
	} {
		tf(t, s)
		teardown()
	}
}

func getTx(sig string, slot uint64) *transaction.Record {
	return &transaction.Record{
		Signature: sig,
		Slot:      slot,
		BlockTime: time.Now(),
		Data:      []byte{0x01, 0x02, 0x03, 0x04},
		HasErrors: true,
	}
}

func testRoundTrip(t *testing.T, s transaction.Store) {
	txId := "test_tx_sig"
	expected := getTx(txId, 123)

	actual, err := s.Get(context.Background(), txId)
	require.Error(t, err)
	assert.Equal(t, transaction.ErrNotFound, err)
	assert.Nil(t, actual)

	err = s.Put(context.Background(), expected)
	require.NoError(t, err)

	err = s.Put(context.Background(), expected)
	require.NoError(t, err)

	actual, err = s.Get(context.Background(), txId)
	require.NoError(t, err)
	assert.Equal(t, expected.Signature, actual.Signature)
	assert.Equal(t, expected.Slot, actual.Slot)
	assert.Equal(t, expected.BlockTime.Unix(), actual.BlockTime.Unix())
	assert.Equal(t, base58.Encode(expected.Data), base58.Encode(actual.Data))
	assert.Equal(t, expected.HasErrors, actual.HasErrors)
	assert.Equal(t, len(expected.TokenBalances), len(actual.TokenBalances))

	txId = "test_tx_sig2"
	fee := uint64(54321)
	expected = getTx(txId, 128)
	expected.Fee = &fee
	expected.TokenBalances = []*transaction.TokenBalance{
		{Account: "account123", PostBalance: 1000},
		{Account: "account543", PreBalance: 999},
		{Account: "account678", PreBalance: 100, PostBalance: 99},
	}

	err = s.Put(context.Background(), expected)
	require.NoError(t, err)

	actual, err = s.Get(context.Background(), txId)
	require.NoError(t, err)
	assert.Equal(t, expected.Signature, actual.Signature)
	assert.Equal(t, expected.Slot, actual.Slot)
	assert.Equal(t, expected.BlockTime.Unix(), actual.BlockTime.Unix())
	assert.Equal(t, base58.Encode(expected.Data), base58.Encode(actual.Data))
	assert.Equal(t, expected.HasErrors, actual.HasErrors)
	assert.Equal(t, len(expected.TokenBalances), 3)

	assert.Equal(t, expected.TokenBalances[0].Account, actual.TokenBalances[0].Account)
	assert.Equal(t, expected.TokenBalances[1].Account, actual.TokenBalances[1].Account)
	assert.Equal(t, expected.TokenBalances[2].Account, actual.TokenBalances[2].Account)

	assert.Equal(t, expected.TokenBalances[0].PreBalance, actual.TokenBalances[0].PreBalance)
	assert.Equal(t, expected.TokenBalances[1].PreBalance, actual.TokenBalances[1].PreBalance)
	assert.Equal(t, expected.TokenBalances[2].PreBalance, actual.TokenBalances[2].PreBalance)

	assert.Equal(t, expected.TokenBalances[0].PostBalance, actual.TokenBalances[0].PostBalance)
	assert.Equal(t, expected.TokenBalances[1].PostBalance, actual.TokenBalances[1].PostBalance)
	assert.Equal(t, expected.TokenBalances[2].PostBalance, actual.TokenBalances[2].PostBalance)

	txId = "test_tx_sig3"
	fee = uint64(321)
	expected = getTx(txId, 139)
	expected.TokenBalances = []*transaction.TokenBalance{
		{Account: "account123", PreBalance: 1, PostBalance: 1001},
		{Account: "account543", PreBalance: 1001, PostBalance: 1},
	}

	err = s.Put(context.Background(), expected)
	require.NoError(t, err)

	actual, err = s.Get(context.Background(), txId)
	require.NoError(t, err)
	assert.Equal(t, expected.Signature, actual.Signature)
	assert.Equal(t, expected.Slot, actual.Slot)
	assert.Equal(t, expected.BlockTime.Unix(), actual.BlockTime.Unix())
	assert.Equal(t, base58.Encode(expected.Data), base58.Encode(actual.Data))
	assert.Equal(t, expected.HasErrors, actual.HasErrors)
	assert.Equal(t, len(expected.TokenBalances), 2)

	assert.Equal(t, expected.TokenBalances[0].Account, actual.TokenBalances[0].Account)
	assert.Equal(t, expected.TokenBalances[1].Account, actual.TokenBalances[1].Account)

	assert.Equal(t, expected.TokenBalances[0].PreBalance, actual.TokenBalances[0].PreBalance)
	assert.Equal(t, expected.TokenBalances[1].PreBalance, actual.TokenBalances[1].PreBalance)

	assert.Equal(t, expected.TokenBalances[0].PostBalance, actual.TokenBalances[0].PostBalance)
	assert.Equal(t, expected.TokenBalances[1].PostBalance, actual.TokenBalances[1].PostBalance)

	actual.Confirmations = 32
	err = s.Put(context.Background(), actual)
	require.NoError(t, err)
	actual, err = s.Get(context.Background(), txId)
	require.NoError(t, err)
	assert.Equal(t, uint64(32), actual.Confirmations)

	sigs, err := s.GetSignaturesByState(context.Background(), transaction.ConfirmationUnknown, 100, query.Ascending)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(sigs))

	sigs, err = s.GetSignaturesByState(context.Background(), transaction.ConfirmationUnknown, 1, query.Ascending)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(sigs))

	txs, err := s.GetAllByAddress(context.Background(), "account123", 0, 100, query.Descending)
	/*
		for _, tx := range txs {
			fmt.Printf("tx: %#v\n", tx)
		}
	*/

	assert.NoError(t, err)
	assert.Equal(t, 2, len(txs))
	assert.Equal(t, "test_tx_sig3", txs[0].Signature)
	assert.Equal(t, "test_tx_sig2", txs[1].Signature)

	txs, err = s.GetAllByAddress(context.Background(), "account123", 0, 100, query.Ascending)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(txs))
	assert.Equal(t, "test_tx_sig2", txs[0].Signature)
	assert.Equal(t, "test_tx_sig3", txs[1].Signature)

	txs, err = s.GetAllByAddress(context.Background(), "account123", txs[0].Id, 100, query.Ascending)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(txs))
	assert.Equal(t, "test_tx_sig3", txs[0].Signature)

	txs, err = s.GetAllByAddress(context.Background(), "account123", txs[0].Id, 100, query.Descending)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(txs))
	assert.Equal(t, "test_tx_sig2", txs[0].Signature)

	txs, err = s.GetAllByAddress(context.Background(), "account678", 0, 100, query.Ascending)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(txs))
	assert.Equal(t, "test_tx_sig2", txs[0].Signature)

	_, err = s.GetLatestByState(context.Background(), "account123", transaction.ConfirmationFinalized)
	assert.Error(t, err)

	txId = "test_tx_sig4"
	expected = getTx(txId, 152)
	expected.TokenBalances = []*transaction.TokenBalance{
		{Account: "account123", PreBalance: 1001, PostBalance: 9000},
		{Account: "account888", PreBalance: 9000, PostBalance: 0},
	}
	expected.ConfirmationState = transaction.ConfirmationFinalized

	err = s.Put(context.Background(), expected)
	require.NoError(t, err)

	actual, err = s.GetLatestByState(context.Background(), "account123", transaction.ConfirmationFinalized)
	assert.NoError(t, err)
	assert.Equal(t, "test_tx_sig4", actual.Signature)

	actual, err = s.GetFirstPending(context.Background(), "account123")
	assert.NoError(t, err)
	assert.Equal(t, "test_tx_sig2", actual.Signature)
}
