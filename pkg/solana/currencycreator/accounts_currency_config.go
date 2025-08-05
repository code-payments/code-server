package currencycreator

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	DefaultMintMaxSupply = 21_000_000_000_000
	DefaultMintDecimals  = 6
)

const (
	MaxCurrencyConfigAccountNameLength   = 32
	MaxCurrencyConfigAccountSymbolLength = 8
)

const (
	CurrencyConfigAccountSize = (8 + //discriminator
		32 + // authority
		32 + // creator
		32 + // mint
		MaxCurrencyConfigAccountNameLength + // name
		MaxCurrencyConfigAccountSymbolLength + // symbol
		32 + // seed
		8 + // max_supply
		8 + // current_supply
		1 + // decimal_places
		1 + // bump
		1 + // mint_bump
		5) // padding
)

var CurrencyConfigAccountDiscriminator = []byte{byte(AccountTypeCurrencyConfig), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type CurrencyConfigAccount struct {
	Authority     ed25519.PublicKey
	Creator       ed25519.PublicKey
	Mint          ed25519.PublicKey
	Name          string
	Symbol        string
	Seed          ed25519.PublicKey
	MaxSupply     uint64
	CurrentSupply uint64
	DecimalPlaces uint8
	Bump          uint8
	MintBump      uint8
}

func (obj *CurrencyConfigAccount) Unmarshal(data []byte) error {
	if len(data) < CurrencyConfigAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, CurrencyConfigAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Authority, &offset)
	getKey(data, &obj.Creator, &offset)
	getKey(data, &obj.Mint, &offset)
	getFixedString(data, &obj.Name, MaxCurrencyConfigAccountNameLength, &offset)
	getFixedString(data, &obj.Symbol, MaxCurrencyConfigAccountSymbolLength, &offset)
	getKey(data, &obj.Seed, &offset)
	getUint64(data, &obj.MaxSupply, &offset)
	getUint64(data, &obj.CurrentSupply, &offset)
	getUint8(data, &obj.DecimalPlaces, &offset)
	getUint8(data, &obj.Bump, &offset)
	getUint8(data, &obj.MintBump, &offset)
	offset += 5 // padding

	return nil
}

func (obj *CurrencyConfigAccount) String() string {
	return fmt.Sprintf(
		"CurrencyConfig{authority=%s,creator=%s,mint=%s,name=%s,symbol=%s,seed=%s,max_supply=%d,currency_supply=%d,decimal_places=%d,bump=%d,mint_bump=%d}",
		base58.Encode(obj.Authority),
		base58.Encode(obj.Creator),
		base58.Encode(obj.Mint),
		obj.Name,
		obj.Symbol,
		base58.Encode(obj.Seed),
		obj.MaxSupply,
		obj.CurrentSupply,
		obj.DecimalPlaces,
		obj.Bump,
		obj.MintBump,
	)
}
