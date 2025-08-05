package currencycreator

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/mr-tron/base58"
)

const (
	LiquidityPoolAccountSize = (8 + //discriminator
		32 + // authority
		32 + // currency
		32 + // target_mint
		32 + // base_mint
		32 + // vault_target
		32 + // vault_base
		32 + // fee_target
		32 + // fee_base
		4 + // buy_fee
		4 + // sell_fee
		8 + // created_unix_time
		8 + // go_live_unix_time
		8 + // purchase_cap
		8 + // sale_cap
		RawExponentialCurveSize + // curve
		8 + // supply_from_baseonding
		1 + // bump
		1 + // currency_baseump
		1 + // vault_target_baseump
		1 + // vault_base_baseump
		4) // padding
)

var LiquidityPoolAccountDiscriminator = []byte{byte(AccountTypeLiquidityPool), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type LiquidityPoolAccount struct {
	Authority         ed25519.PublicKey
	Currency          ed25519.PublicKey
	TargetMint        ed25519.PublicKey
	BaseMint          ed25519.PublicKey
	VaultTarget       ed25519.PublicKey
	VaultBase         ed25519.PublicKey
	FeeTarget         ed25519.PublicKey
	FeeBase           ed25519.PublicKey
	BuyFee            uint32
	SellFee           uint32
	CreatedUnixTime   int64
	GoLiveUnixTime    int64
	PurchaseCap       uint64
	SaleCap           uint64
	Curve             RawExponentialCurve
	SupplyFromBonding uint64
	Bump              uint8
	CurrencyBump      uint8
	VaultTargetBump   uint8
	VaultBaseBump     uint8
}

func (obj *LiquidityPoolAccount) Unmarshal(data []byte) error {
	if len(data) < LiquidityPoolAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, LiquidityPoolAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Authority, &offset)
	getKey(data, &obj.Currency, &offset)
	getKey(data, &obj.TargetMint, &offset)
	getKey(data, &obj.BaseMint, &offset)
	getKey(data, &obj.VaultTarget, &offset)
	getKey(data, &obj.VaultBase, &offset)
	getKey(data, &obj.FeeTarget, &offset)
	getKey(data, &obj.FeeBase, &offset)
	getUint32(data, &obj.BuyFee, &offset)
	getUint32(data, &obj.SellFee, &offset)
	getInt64(data, &obj.CreatedUnixTime, &offset)
	getInt64(data, &obj.GoLiveUnixTime, &offset)
	getUint64(data, &obj.PurchaseCap, &offset)
	getUint64(data, &obj.SaleCap, &offset)
	getRawExponentialCurve(data, &obj.Curve, &offset)
	getUint64(data, &obj.SupplyFromBonding, &offset)
	getUint8(data, &obj.Bump, &offset)
	getUint8(data, &obj.CurrencyBump, &offset)
	getUint8(data, &obj.VaultTargetBump, &offset)
	getUint8(data, &obj.VaultBaseBump, &offset)
	offset += 4 // padding

	return nil
}

func (obj *LiquidityPoolAccount) String() string {
	return fmt.Sprintf(
		"LiquidityPool{authority=%s,currency=%s,target_mint=%s,base_mint=%s,vault_target=%s,vault_base=%s,fee_target=%s,fee_base=%s,buy_fee=%d,sell_fee=%d,created_unix_time=%s,go_live_unix_time=%s,purchase_cap=%d,sale_cap=%d,curve=%s,bump=%d,currency_bump=%d,vault_target_bump=%d,vault_base_bump=%d}",
		base58.Encode(obj.Authority),
		base58.Encode(obj.Currency),
		base58.Encode(obj.TargetMint),
		base58.Encode(obj.BaseMint),
		base58.Encode(obj.VaultTarget),
		base58.Encode(obj.VaultBase),
		base58.Encode(obj.FeeTarget),
		base58.Encode(obj.FeeBase),
		obj.BuyFee,
		obj.SellFee,
		time.Unix(obj.CreatedUnixTime, 0).UTC().String(),
		time.Unix(obj.GoLiveUnixTime, 0).UTC().String(),
		obj.PurchaseCap,
		obj.SaleCap,
		obj.Curve.String(),
		obj.Bump,
		obj.CurrencyBump,
		obj.VaultTargetBump,
		obj.VaultBaseBump,
	)
}
