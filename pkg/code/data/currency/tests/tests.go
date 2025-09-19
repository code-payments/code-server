package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/solana/currencycreator"
)

func RunTests(t *testing.T, s currency.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s currency.Store){
		testExchangeRateRoundTrip,
		testGetExchangeRatesInRange,
		testMetadataRoundTrip,
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

func testMetadataRoundTrip(t *testing.T, s currency.Store) {
	expected := &currency.MetadataRecord{
		Name:        "Jeffy",
		Symbol:      "JFY",
		Description: "A test currency for Flipcash created by Jeff Yanta so we can eat our own dog food as we build out the platform. Pun intended",
		ImageUrl:    "https://flipcash-currency-assets.s3.us-east-1.amazonaws.com/52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj/icon.png",

		Seed: "H7WNaHtCa5h2k7AwZ8DbdLfM6bU2bi2jmWiUkFqgeBYk",

		Authority: "jfy1btcfsjSn2WCqLVaxiEjp4zgmemGyRsdCPbPwnZV",

		Mint:     "52MNGpgvydSwCtC2H4qeiZXZ1TxEuRVCRGa8LAfk2kSj",
		MintBump: 252,
		Decimals: currencycreator.DefaultMintDecimals,

		CurrencyConfig:     "BDfFyqfasvty3cjSbC2qZx2Dmr4vhhVBt9Ban5XsTcEH",
		CurrencyConfigBump: 251,

		LiquidityPool:     "5cH99GSbr9ECP8gd1vLiAAFPHt1VeCNKzzrPFGmAB61c",
		LiquidityPoolBump: 255,

		VaultMint:     "BFDanLgELhpCCGTtaa7c8WGxTXcTxgwkf9DMQd4qheSK",
		VaultMintBump: 255,

		VaultCore:     "A9NVHVuorNL4y2YFxdwdU3Hqozxw1Y1YJ81ZPxJsRrT4",
		VaultCoreBump: 255,

		FeesMint:  "BfWacqZVHQt3VNwPugXAkLrApgCTnjgF6nQb7xEMqeDu",
		BuyFeeBps: currencycreator.DefaultBuyFeeBps,

		FeesCore:   "5EcVYL8jHRKeeQqg6eYVBzc73ecH1PFzzaavoQBKRYy5",
		SellFeeBps: currencycreator.DefaultSellFeeBps,

		CreatedBy: "jyyy4RpW3X5ApbW5G6vx9ZVPxhoUKGRLbZ4LxC47LYG",
		CreatedAt: time.Now(),
	}

	_, err := s.GetMetadata(context.Background(), expected.Mint)
	assert.Equal(t, currency.ErrNotFound, err)

	cloned := expected.Clone()
	require.NoError(t, s.PutMetadata(context.Background(), expected))
	assert.EqualValues(t, 1, expected.Id)

	actual, err := s.GetMetadata(context.Background(), expected.Mint)
	require.NoError(t, err)
	assertEquivalentMetadataRecords(t, cloned, actual)
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

func assertEquivalentMetadataRecords(t *testing.T, obj1, obj2 *currency.MetadataRecord) {
	assert.Equal(t, obj1.Name, obj2.Name)
	assert.Equal(t, obj1.Symbol, obj2.Symbol)
	assert.Equal(t, obj1.Description, obj2.Description)
	assert.Equal(t, obj1.ImageUrl, obj2.ImageUrl)
	assert.Equal(t, obj1.Seed, obj2.Seed)
	assert.Equal(t, obj1.Authority, obj2.Authority)
	assert.Equal(t, obj1.Mint, obj2.Mint)
	assert.Equal(t, obj1.MintBump, obj2.MintBump)
	assert.Equal(t, obj1.Decimals, obj2.Decimals)
	assert.Equal(t, obj1.CurrencyConfig, obj2.CurrencyConfig)
	assert.Equal(t, obj1.CurrencyConfigBump, obj2.CurrencyConfigBump)
	assert.Equal(t, obj1.LiquidityPool, obj2.LiquidityPool)
	assert.Equal(t, obj1.LiquidityPoolBump, obj2.LiquidityPoolBump)
	assert.Equal(t, obj1.VaultMint, obj2.VaultMint)
	assert.Equal(t, obj1.VaultMintBump, obj2.VaultMintBump)
	assert.Equal(t, obj1.VaultCore, obj2.VaultCore)
	assert.Equal(t, obj1.VaultCoreBump, obj2.VaultCoreBump)
	assert.Equal(t, obj1.FeesMint, obj2.FeesMint)
	assert.Equal(t, obj1.BuyFeeBps, obj2.BuyFeeBps)
	assert.Equal(t, obj1.FeesCore, obj2.FeesCore)
	assert.Equal(t, obj1.SellFeeBps, obj2.SellFeeBps)
	assert.Equal(t, obj1.CreatedBy, obj2.CreatedBy)
	assert.Equal(t, obj1.CreatedAt.Unix(), obj2.CreatedAt.Unix())
}
