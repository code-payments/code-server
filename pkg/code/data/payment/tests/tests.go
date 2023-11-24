package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/payment"
)

type TestData struct {
	Timestamp   time.Time
	Source      string
	Destination string

	ExchangeRate     float64
	ExchangeCurrency string

	Quantity        uint64
	BookCost        float64
	MarketValue     float64
	UnrealizedGains float64
}

func CompareTestDataToPayment(t *testing.T, expected *TestData, actual *payment.Record) {
	assert.Equal(t, expected.Timestamp.Unix(), actual.CreatedAt.Unix())
	assert.Equal(t, expected.Source, actual.Source)
	assert.Equal(t, expected.Destination, actual.Destination)
	assert.Equal(t, expected.ExchangeCurrency, actual.ExchangeCurrency)
	assert.Equal(t, expected.ExchangeRate, actual.ExchangeRate)
	assert.Equal(t, expected.Quantity, actual.Quantity)
	assert.Equal(t, expected.MarketValue, actual.UsdMarketValue)
}

func RunTests(t *testing.T, s payment.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s payment.Store){
		testRoundTrip,
		testGetRange,
	} {
		tf(t, s)
		teardown()
	}
}

func testRoundTrip(t *testing.T, s payment.Store) {
	now := time.Now()
	region := "ca"
	data := &payment.Record{
		TransactionId:    "tx_sig",
		TransactionIndex: 0,
		Source:           "source",
		Destination:      "destination",
		Quantity:         100,
		ExchangeCurrency: "cad",
		Region:           &region,
		ExchangeRate:     100.0,
		IsWithdraw:       true,
		CreatedAt:        now,
	}

	// Test ErrTransactionSignatureAndIndexNotFound
	actualData, err := s.GetAllForAccount(context.Background(), "foobar", 0, 1, query.Ascending)
	//actualData, err := s.GetBySignature(context.Background(), "foobar", 0)
	assert.Equal(t, payment.ErrNotFound, err)
	assert.Nil(t, actualData)

	require.NoError(t, s.Put(context.Background(), data))

	// Test Err
	assert.Equal(t, payment.ErrExists, s.Put(context.Background(), data))

	actualData, err = s.GetAllForAccount(context.Background(), data.Source, 0, 1, query.Ascending)
	//actualData, err = s.GetBySignature(context.Background(), data.TransactionSignature, data.TransactionIndex)
	require.NoError(t, err)
	assert.Equal(t, 1, len(actualData))
	assert.Equal(t, data.TransactionId, actualData[0].TransactionId)
	assert.Equal(t, data.TransactionIndex, actualData[0].TransactionIndex)
	assert.Equal(t, data.Source, actualData[0].Source)
	assert.Equal(t, data.Destination, actualData[0].Destination)
	assert.Equal(t, data.Quantity, actualData[0].Quantity)
	assert.Equal(t, data.ExchangeCurrency, actualData[0].ExchangeCurrency)
	assert.Equal(t, *data.Region, *actualData[0].Region)
	assert.Equal(t, data.ExchangeRate, actualData[0].ExchangeRate)
	assert.Equal(t, data.IsWithdraw, actualData[0].IsWithdraw)
	assert.Equal(t, data.CreatedAt.Unix(), actualData[0].CreatedAt.Unix())
}

func testGetRange(t *testing.T, s payment.Store) {
	joe := "joe"
	eric := "eric"
	tim := "tim"
	emma := "emma"
	sub := "sub"

	now := time.Now()
	times := []time.Time{
		now.Add(-27 * time.Minute),
		now.Add(-18 * time.Minute),
		now.Add(-10 * time.Minute),
		now.Add(-5 * time.Minute),
		now.Add(-3 * time.Minute),
		now,
	}

	testPayments := []*TestData{
		{Timestamp: times[0], Source: eric, Destination: joe, ExchangeCurrency: "cad", ExchangeRate: 0.0000483, Quantity: 103498, BookCost: 5.00, MarketValue: 5.00, UnrealizedGains: 0.00},
		{Timestamp: times[1], Source: joe, Destination: eric, ExchangeCurrency: "cad", ExchangeRate: 0.0000661, Quantity: 88358, BookCost: 4.00, MarketValue: 5.84, UnrealizedGains: 1.84},
		{Timestamp: times[2], Source: tim, Destination: joe, ExchangeCurrency: "cad", ExchangeRate: 0.0000611, Quantity: 248175, BookCost: 13.76, MarketValue: 15.16, UnrealizedGains: 1.40},
		{Timestamp: times[3], Source: joe, Destination: emma, ExchangeCurrency: "cad", ExchangeRate: 0.0000662, Quantity: 184239, BookCost: 9.53, MarketValue: 12.19, UnrealizedGains: 2.66},
		{Timestamp: times[4], Source: sub, Destination: joe, ExchangeCurrency: "cad", ExchangeRate: 0.0000935, Quantity: 317886, BookCost: 22.03, MarketValue: 29.73, UnrealizedGains: 7.70},
		{Timestamp: times[5], Source: sub, Destination: joe, ExchangeCurrency: "cad", ExchangeRate: 0.0001286, Quantity: 415064, BookCost: 34.53, MarketValue: 53.39, UnrealizedGains: 18.86},
	}

	for index, item := range testPayments {
		err := s.Put(context.Background(), &payment.Record{
			TransactionIndex: 0,
			TransactionId:    fmt.Sprintf("tx_sig__%d", index),

			Source:      item.Source,      // The source account id for this payment
			Destination: item.Destination, // The destination account id for this payment
			Quantity:    item.Quantity,    // The amount of Kin (in Quarks)

			ExchangeCurrency: item.ExchangeCurrency, // The (external) agreed upon currency for the exchange
			ExchangeRate:     item.ExchangeRate,     // The (external) agreed upon exchange rate for determining the amount of Kin to transfer

			UsdMarketValue: item.MarketValue, // Not accurate, but fine for testing

			CreatedAt: item.Timestamp,
		})

		require.NoError(t, err)
	}

	// GetAllForAccountWithCursor Tests
	// --------------------------------------------------------------------------------

	results, err := s.GetAllForAccount(context.Background(), joe, 4, 100, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, 2, len(results))
	CompareTestDataToPayment(t, testPayments[4], results[0])
	CompareTestDataToPayment(t, testPayments[5], results[1])

}
