package currencycreator

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	DefaultMintMaxTokenSupply = 21_000_000 // 21mm tokens
	DefaultMintQuarksPerUnit  = 10_000_000_000
	DefaultMintMaxQuarkSupply = DefaultMintMaxTokenSupply * DefaultMintQuarksPerUnit // 21mm tokens with 10 decimals
	DefaultMintDecimals       = 10
)

const (
	MaxCurrencyConfigAccountNameLength   = 32
	MaxCurrencyConfigAccountSymbolLength = 8
)

const (
	CurrencyConfigAccountSize = (8 + //discriminator
		32 + // authority
		32 + // mint
		MaxCurrencyConfigAccountNameLength + // name
		MaxCurrencyConfigAccountSymbolLength + // symbol
		32 + // seed
		1 + // bump
		1 + // mint_bump
		6) // padding
)

var CurrencyConfigAccountDiscriminator = []byte{byte(AccountTypeCurrencyConfig), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type CurrencyConfigAccount struct {
	Authority ed25519.PublicKey
	Mint      ed25519.PublicKey
	Name      string
	Symbol    string
	Seed      ed25519.PublicKey
	Bump      uint8
	MintBump  uint8
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
	getKey(data, &obj.Mint, &offset)
	getFixedString(data, &obj.Name, MaxCurrencyConfigAccountNameLength, &offset)
	getFixedString(data, &obj.Symbol, MaxCurrencyConfigAccountSymbolLength, &offset)
	getKey(data, &obj.Seed, &offset)
	getUint8(data, &obj.Bump, &offset)
	getUint8(data, &obj.MintBump, &offset)
	offset += 6 // padding

	return nil
}

func (obj *CurrencyConfigAccount) String() string {
	return fmt.Sprintf(
		"CurrencyConfig{authority=%s,mint=%s,name=%s,symbol=%s,seed=%s,bump=%d,mint_bump=%d}",
		base58.Encode(obj.Authority),
		base58.Encode(obj.Mint),
		obj.Name,
		obj.Symbol,
		base58.Encode(obj.Seed),
		obj.Bump,
		obj.MintBump,
	)
}
