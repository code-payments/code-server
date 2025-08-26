package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/database/query"
)

func RunTests(t *testing.T, s currency.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s currency.Store){
		testExchangeRateRoundTrip,
		testGetExchangeRatesInRange,
		testReserveRoundTrip,
	} {
		tf(t, s)
		teardown()
	}
}

func testExchangeRateRoundTrip(t *testing.T, s currency.Store) {
	now := time.Date(2021, 01, 29, 13, 0, 5, 0, time.UTC)

	record, err := s.GetAllExchangeRates(context.Background(), now)
	assert.Nil(t, record)
	assert.Equal(t, currency.ErrNotFound, err)

	rates := map[string]float64{
		"usd": 0.000055,
		"cad": 0.00007,
	}
	require.NoError(t, s.PutExchangeRates(context.Background(), &currency.MultiRateRecord{
		Time:  now,
		Rates: rates,
	}))

	// Overwrite should fail
	assert.Equal(t, currency.ErrExists, s.PutExchangeRates(context.Background(), &currency.MultiRateRecord{
		Time:  now,
		Rates: rates,
	}))

	// Test GetExchangeRate(), it should return the USD record
	single, err := s.GetExchangeRate(context.Background(), "usd", now)
	require.NoError(t, err)
	assert.Equal(t, now.Unix(), single.Time.Unix())
	assert.EqualValues(t, rates["usd"], single.Rate)

	// Test GetAllExchangeRates(), it should return all recent rates
	record, err = s.GetAllExchangeRates(context.Background(), now)
	require.NoError(t, err)

	assert.Equal(t, now.Unix(), record.Time.Unix())
	assert.EqualValues(t, rates, record.Rates)

	// within same day, should return entry
	record, err = s.GetAllExchangeRates(context.Background(), time.Date(2021, 01, 29, 14, 0, 5, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, now.Unix(), record.Time.Unix())
	assert.EqualValues(t, rates, record.Rates)

	// day after, should be empty
	tomorrow := time.Date(2021, 01, 30, 0, 0, 0, 0, time.UTC)
	record, err = s.GetAllExchangeRates(context.Background(), tomorrow)
	assert.Nil(t, record)
	assert.Equal(t, currency.ErrNotFound, err)
}

func testGetExchangeRatesInRange(t *testing.T, s currency.Store) {
	var rates []currency.MultiRateRecord

	now := time.Now().UTC()

	for i := 0; i < 100; i++ {
		rates = append(rates, currency.MultiRateRecord{
			Time: now.Add(time.Duration(i) * time.Hour),
			Rates: map[string]float64{
				"usd": (0.000058 + float64(i/10000)),
				"cad": (0.00008 + float64(i/10000)),
			},
		})
	}

	record, err := s.GetAllExchangeRates(context.Background(), rates[0].Time)
	assert.Nil(t, record)
	assert.Equal(t, currency.ErrNotFound, err)

	for _, item := range rates {
		require.NoError(t, s.PutExchangeRates(context.Background(), &item))
	}

	result, err := s.GetExchangeRatesInRange(context.Background(), "usd", query.IntervalRaw, rates[0].Time, rates[99].Time, query.Ascending)
	require.NoError(t, err)
	assert.Equal(t, len(result), 100)

	for i, item := range result {
		assert.Equal(t, rates[i].Time.Unix(), item.Time.Unix())
		assert.EqualValues(t, rates[i].Rates["usd"], item.Rate)
	}

	_, err = s.GetExchangeRatesInRange(context.Background(), "usd", query.IntervalHour, rates[0].Time, rates[99].Time, query.Ascending)
	require.NoError(t, err)
	_, err = s.GetExchangeRatesInRange(context.Background(), "usd", query.IntervalDay, rates[0].Time, rates[99].Time, query.Ascending)
	require.NoError(t, err)
	_, err = s.GetExchangeRatesInRange(context.Background(), "usd", query.IntervalWeek, rates[0].Time, rates[99].Time, query.Ascending)
	require.NoError(t, err)
	_, err = s.GetExchangeRatesInRange(context.Background(), "usd", query.IntervalMonth, rates[0].Time, rates[99].Time, query.Ascending)
	require.NoError(t, err)
}

func testReserveRoundTrip(t *testing.T, s currency.Store) {
	now := time.Date(2021, 01, 29, 13, 0, 5, 0, time.UTC)

	record, err := s.GetReserveAtTime(context.Background(), "mint", now)
	assert.Nil(t, record)
	assert.Equal(t, currency.ErrNotFound, err)

	expected := &currency.ReserveRecord{
		Mint:              "mint",
		SupplyFromBonding: 1,
		CoreMintLocked:    2,
		Time:              now,
	}
	require.NoError(t, s.PutReserveRecord(context.Background(), expected))

	assert.Equal(t, currency.ErrExists, s.PutReserveRecord(context.Background(), expected))

	actual, err := s.GetReserveAtTime(context.Background(), "mint", now)
	require.NoError(t, err)
	assert.Equal(t, now.Unix(), actual.Time.Unix())
	assert.Equal(t, actual.SupplyFromBonding, expected.SupplyFromBonding)
	assert.Equal(t, actual.CoreMintLocked, expected.CoreMintLocked)

	actual, err = s.GetReserveAtTime(context.Background(), "mint", time.Date(2021, 01, 29, 14, 0, 5, 0, time.UTC))
	require.NoError(t, err)

	assert.Equal(t, now.Unix(), actual.Time.Unix())
	assert.Equal(t, actual.SupplyFromBonding, expected.SupplyFromBonding)
	assert.Equal(t, actual.CoreMintLocked, expected.CoreMintLocked)

	tomorrow := time.Date(2021, 01, 30, 0, 0, 0, 0, time.UTC)
	actual, err = s.GetReserveAtTime(context.Background(), "mint", tomorrow)
	assert.Nil(t, actual)
	assert.Equal(t, currency.ErrNotFound, err)
}
