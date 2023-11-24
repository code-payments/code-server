package data

import (
	pg "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	maxAccountReqSize            = 1024
	maxBalanceHistoryReqSize     = 1024
	maxCurrencyHistoryReqSize    = 1024
	maxEventReqSize              = 1024
	maxPaymentHistoryReqSize     = 1024
	maxTransactionHistoryReqSize = 1024
	maxTimelockBatchReqSize      = 1024
)

type Provider interface {
	BlockchainData
	DatabaseData
	WebData
	EstimatedData

	GetBlockchainDataProvider() BlockchainData
	GetDatabaseDataProvider() DatabaseData
	GetWebDataProvider() WebData
	GetEstimatedDataProvider() EstimatedData
}

type DataProvider struct {
	*BlockchainProvider
	*DatabaseProvider
	*WebProvider
	*EstimatedProvider
}

func NewDataProvider(dbConfig *pg.Config, solanaEnv string, configProvider ConfigProvider) (Provider, error) {
	blockchain, err := NewBlockchainProvider(solanaEnv)
	if err != nil {
		return nil, err
	}

	p, err := NewDataProviderWithoutBlockchain(dbConfig, configProvider)
	if err != nil {
		return nil, err
	}

	provider := p.(*DataProvider)
	provider.BlockchainProvider = blockchain.(*BlockchainProvider)

	return provider, nil
}

func NewDataProviderWithoutBlockchain(dbConfig *pg.Config, configProvider ConfigProvider) (Provider, error) {
	db, err := NewDatabaseProvider(dbConfig)
	if err != nil {
		return nil, err
	}

	web, err := NewWebProvider(configProvider)
	if err != nil {
		return nil, err
	}

	estimated, err := NewEstimatedProvider()
	if err != nil {
		return nil, err
	}

	provider := &DataProvider{
		DatabaseProvider:  db.(*DatabaseProvider),
		WebProvider:       web.(*WebProvider),
		EstimatedProvider: estimated.(*EstimatedProvider),
	}

	return provider, nil
}

func NewTestDataProvider() Provider {
	// todo: This currently only includes database data and should include the
	//       other provider types.

	blockchain, err := NewBlockchainProvider("https://api.testnet.solana.com")
	if err != nil {
		panic(err)
	}

	return &DataProvider{
		DatabaseProvider:   NewTestDatabaseProvider().(*DatabaseProvider),
		BlockchainProvider: blockchain.(*BlockchainProvider),
	}
}

func (p *DataProvider) GetBlockchainDataProvider() BlockchainData {
	return p.BlockchainProvider
}
func (p *DataProvider) GetWebDataProvider() WebData {
	return p.WebProvider
}
func (p *DataProvider) GetDatabaseDataProvider() DatabaseData {
	return p.DatabaseProvider
}
func (p *DataProvider) GetEstimatedDataProvider() EstimatedData {
	return p.EstimatedProvider
}
