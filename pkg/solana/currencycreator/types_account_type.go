package currencycreator

type AccountType uint8

const (
	AccountTypeUnknown AccountType = iota
	AccountTypeCurrencyConfig
	AccountTypeLiquidityPool
)
