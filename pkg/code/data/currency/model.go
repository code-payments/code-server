package currency

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/solana/currencycreator"
)

type ExchangeRateRecord struct {
	Id     uint64
	Time   time.Time
	Rate   float64
	Symbol string
}

type MultiRateRecord struct {
	Time  time.Time
	Rates map[string]float64
}

type MetadataRecord struct {
	Id uint64

	Name   string
	Symbol string

	Seed string

	Authority string

	Mint     string
	MintBump uint8
	Decimals uint8

	CurrencyConfig     string
	CurrencyConfigBump uint8

	LiquidityPool     string
	LiquidityPoolBump uint8

	VaultMint     string
	VaultMintBump uint8

	VaultCore     string
	VaultCoreBump uint8

	FeesMint  string
	BuyFeeBps uint16

	FeesCore   string
	SellFeeBps uint16

	CreatedBy string
	CreatedAt time.Time
}

func (m *MetadataRecord) Validate() error {
	if len(m.Name) == 0 {
		return errors.New("name is required")
	}

	if len(m.Symbol) == 0 {
		return errors.New("symbol is required")
	}

	if len(m.Seed) == 0 {
		return errors.New("seed is required")
	}

	if len(m.Authority) == 0 {
		return errors.New("authority is required")
	}

	if len(m.Mint) == 0 {
		return errors.New("mint is required")
	}

	if m.MintBump == 0 {
		return errors.New("mint bump is required")
	}

	if m.Decimals != currencycreator.DefaultMintDecimals {
		return errors.New("invalid mint decimals")
	}

	if len(m.CurrencyConfig) == 0 {
		return errors.New("currency config is required")
	}

	if m.CurrencyConfigBump == 0 {
		return errors.New("currency config bump is required")
	}

	if len(m.LiquidityPool) == 0 {
		return errors.New("liquidity pool is required")
	}

	if m.LiquidityPoolBump == 0 {
		return errors.New("liquidity pool bump is required")
	}

	if len(m.VaultMint) == 0 {
		return errors.New("vault mint is required")
	}

	if m.VaultMintBump == 0 {
		return errors.New("vault mint bump is required")
	}

	if len(m.VaultCore) == 0 {
		return errors.New("vault core is required")
	}

	if m.VaultCoreBump == 0 {
		return errors.New("vault core bump is required")
	}

	if len(m.FeesMint) == 0 {
		return errors.New("fees mint is required")
	}

	if m.BuyFeeBps != currencycreator.DefaultBuyFeeBps {
		return errors.New("invalid buy fee bps")
	}

	if len(m.Name) == 0 {
		return errors.New("fees core is required")
	}

	if m.SellFeeBps != currencycreator.DefaultSellFeeBps {
		return errors.New("invalid buy sell bps")
	}

	if len(m.CreatedBy) == 0 {
		return errors.New("created by is required")
	}

	if m.CreatedAt.IsZero() {
		return errors.New("creation timestamp is required")
	}

	return nil
}

func (m *MetadataRecord) Clone() *MetadataRecord {
	return &MetadataRecord{
		Id: m.Id,

		Name:   m.Name,
		Symbol: m.Symbol,

		Seed: m.Seed,

		Authority: m.Authority,

		Mint:     m.Mint,
		MintBump: m.MintBump,
		Decimals: m.Decimals,

		CurrencyConfig:     m.CurrencyConfig,
		CurrencyConfigBump: m.CurrencyConfigBump,

		LiquidityPool:     m.LiquidityPool,
		LiquidityPoolBump: m.LiquidityPoolBump,

		VaultMint:     m.VaultMint,
		VaultMintBump: m.VaultMintBump,

		VaultCore:     m.VaultCore,
		VaultCoreBump: m.VaultCoreBump,

		FeesMint:  m.FeesMint,
		BuyFeeBps: m.BuyFeeBps,

		FeesCore:   m.FeesCore,
		SellFeeBps: m.SellFeeBps,

		CreatedBy: m.CreatedBy,
		CreatedAt: m.CreatedAt,
	}
}

func (m *MetadataRecord) CopyTo(dst *MetadataRecord) {
	dst.Id = m.Id

	dst.Name = m.Name
	dst.Symbol = m.Symbol

	dst.Seed = m.Seed

	dst.Authority = m.Authority

	dst.Mint = m.Mint
	dst.MintBump = m.MintBump
	dst.Decimals = m.Decimals

	dst.CurrencyConfig = m.CurrencyConfig
	dst.CurrencyConfigBump = m.CurrencyConfigBump

	dst.LiquidityPool = m.LiquidityPool
	dst.LiquidityPoolBump = m.LiquidityPoolBump

	dst.VaultMint = m.VaultMint
	dst.VaultMintBump = m.VaultMintBump

	dst.VaultCore = m.VaultCore
	dst.VaultCoreBump = m.VaultCoreBump

	dst.FeesMint = m.FeesMint
	dst.BuyFeeBps = m.BuyFeeBps

	dst.FeesCore = m.FeesCore
	dst.SellFeeBps = m.SellFeeBps

	dst.CreatedBy = m.CreatedBy
	dst.CreatedAt = m.CreatedAt
}

type ReserveRecord struct {
	Id                uint64
	Mint              string
	SupplyFromBonding uint64
	CoreMintLocked    uint64
	Time              time.Time
}

func (m *ReserveRecord) Validate() error {
	if len(m.Mint) == 0 {
		return errors.New("mint is required")
	}

	if m.Time.IsZero() {
		return errors.New("timestamp is required")
	}

	return nil
}

func (m *ReserveRecord) Clone() *ReserveRecord {
	return &ReserveRecord{
		Id:                m.Id,
		Mint:              m.Mint,
		SupplyFromBonding: m.SupplyFromBonding,
		CoreMintLocked:    m.CoreMintLocked,
		Time:              m.Time,
	}
}

func (m *ReserveRecord) CopyTo(dst *ReserveRecord) {
	dst.Id = m.Id
	dst.Mint = m.Mint
	dst.SupplyFromBonding = m.SupplyFromBonding
	dst.CoreMintLocked = m.CoreMintLocked
	dst.Time = m.Time
}
