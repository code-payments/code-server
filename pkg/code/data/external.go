package data

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/code/config"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/currency/coingecko"
	"github.com/code-payments/code-server/pkg/currency/fixer"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/usdc"
)

const (
	webProviderMetricsName = "data.web_provider"
)

type WebData interface {

	// Currency
	// --------------------------------------------------------------------------------

	GetCurrentExchangeRatesFromExternalProviders(ctx context.Context) (*currency.MultiRateRecord, error)
	GetPastExchangeRatesFromExternalProviders(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error)
}

type WebProvider struct {
	coinGecko currency_lib.Client
	fixer     currency_lib.Client
}

func NewWebProvider(configProvider ConfigProvider) (WebData, error) {
	conf := configProvider()
	return &WebProvider{
		coinGecko: coingecko.NewClient(),
		fixer:     fixer.NewClient(conf.fixerApiKey.Get(context.Background())),
	}, nil
}

// Currency
// --------------------------------------------------------------------------------
func (dp *WebProvider) GetCurrentExchangeRatesFromExternalProviders(ctx context.Context) (*currency.MultiRateRecord, error) {
	tracer := metrics.TraceMethodCall(ctx, webProviderMetricsName, "GetCurrentExchangeRatesFromExternalProviders")
	defer tracer.End()

	coinGeckoData, err := dp.coinGecko.GetCurrentRates(ctx, string(config.CoreMintSymbol))
	if err != nil {
		return nil, err
	}

	fixerData, err := dp.fixer.GetCurrentRates(ctx, string(currency_lib.USD))
	if err != nil {
		return nil, err
	}

	rates, err := computeAllExchangeRates(coinGeckoData.Rates, fixerData.Rates)
	if err != nil {
		return nil, err
	}

	return &currency.MultiRateRecord{
		Time:  time.Now(),
		Rates: rates,
	}, nil
}
func (dp *WebProvider) GetPastExchangeRatesFromExternalProviders(ctx context.Context, t time.Time) (*currency.MultiRateRecord, error) {
	tracer := metrics.TraceMethodCall(ctx, webProviderMetricsName, "GetPastExchangeRatesFromExternalProviders")
	defer tracer.End()

	coinGeckoData, err := dp.coinGecko.GetHistoricalRates(ctx, string(config.CoreMintSymbol), t.UTC())
	if err != nil {
		return nil, err
	}

	fixerData, err := dp.fixer.GetHistoricalRates(ctx, string(currency_lib.USD), t.UTC())
	if err != nil {
		return nil, err
	}

	rates, err := computeAllExchangeRates(coinGeckoData.Rates, fixerData.Rates)
	if err != nil {
		return nil, err
	}

	return &currency.MultiRateRecord{
		Time:  coinGeckoData.Timestamp,
		Rates: rates,
	}, nil
}

func computeAllExchangeRates(coreMintRates map[string]float64, usdRates map[string]float64) (map[string]float64, error) {
	coreMintToUsd, ok := coreMintRates[string(currency_lib.USD)]
	if !ok {
		return nil, errors.New("usd rate missing")
	}
	if config.CoreMintPublicKeyString == usdc.Mint {
		coreMintToUsd = 1.0
	}

	res := make(map[string]float64)
	res[string(currency_lib.USD)] = coreMintToUsd
	for symbol, usdRate := range usdRates {
		coreExchangeRate := usdRate * coreMintToUsd
		res[symbol] = coreExchangeRate
	}
	return res, nil
}
