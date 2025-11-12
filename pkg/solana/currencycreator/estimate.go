package currencycreator

import (
	"math"
	"math/big"
)

func EstimateCurrentPrice(currentSupplyInQuarks uint64) *big.Float {
	scale := big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledCurrentSupply := big.NewFloat(float64(currentSupplyInQuarks)).SetPrec(defaultCurvePrec)
	scaledCurrentSupply := new(big.Float).Quo(unscaledCurrentSupply, scale)
	return DefaultExponentialCurve().SpotPriceAtSupply(scaledCurrentSupply)
}

type EstimateValueExchangeArgs struct {
	ValueInQuarks        uint64
	CurrentValueInQuarks uint64
	ValueMintDecimals    uint8
}

func EstimateValueExchange(args *EstimateValueExchangeArgs) uint64 {
	scale := big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledValue := big.NewFloat(float64(args.ValueInQuarks)).SetPrec(defaultCurvePrec)
	scaledValue := new(big.Float).Quo(unscaledValue, scale)

	scale = big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledCurrentValue := big.NewFloat(float64(args.CurrentValueInQuarks)).SetPrec(defaultCurvePrec)
	scaledCurrentValue := new(big.Float).Quo(unscaledCurrentValue, scale)

	scale = big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	scaledTokens := DefaultExponentialCurve().TokensForValueExchange(scaledCurrentValue, scaledValue)
	unscaledTokens := new(big.Float).Mul(scaledTokens, scale)

	quarks, _ := unscaledTokens.Int64()
	return uint64(quarks)
}

type EstimateBuyArgs struct {
	BuyAmountInQuarks     uint64
	CurrentSupplyInQuarks uint64
	ValueMintDecimals     uint8
	BuyFeeBps             uint16
}

func EstimateBuy(args *EstimateBuyArgs) (uint64, uint64) {
	scale := big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledBuyAmount := big.NewFloat(float64(args.BuyAmountInQuarks)).SetPrec(defaultCurvePrec)
	scaledBuyAmount := new(big.Float).Quo(unscaledBuyAmount, scale)

	scale = big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledCurrentSupply := big.NewFloat(float64(args.CurrentSupplyInQuarks)).SetPrec(defaultCurvePrec)
	scaledCurrentSupply := new(big.Float).Quo(unscaledCurrentSupply, scale)

	scale = big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	scaledTokens := DefaultExponentialCurve().TokensBoughtForValue(scaledCurrentSupply, scaledBuyAmount)
	unscaledTokens := new(big.Float).Mul(scaledTokens, scale)

	scale = big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	feePctValue := new(big.Float).SetPrec(defaultCurvePrec).Quo(big.NewFloat(float64(args.BuyFeeBps)), big.NewFloat(10000))
	scaledFees := new(big.Float).Mul(scaledTokens, feePctValue)
	unscaledFees := new(big.Float).Mul(scaledFees, scale)

	tokens, _ := unscaledTokens.Int64()
	fees, _ := unscaledFees.Int64()
	return uint64(tokens - fees), uint64(fees)
}

type EstimateSellArgs struct {
	SellAmountInQuarks   uint64
	CurrentValueInQuarks uint64
	ValueMintDecimals    uint8
	SellFeeBps           uint16
}

func EstimateSell(args *EstimateSellArgs) (uint64, uint64) {
	scale := big.NewFloat(math.Pow10(int(DefaultMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledSellAmount := big.NewFloat(float64(args.SellAmountInQuarks)).SetPrec(defaultCurvePrec)
	scaledSellAmount := new(big.Float).Quo(unscaledSellAmount, scale)

	scale = big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	unscaledCurrentValue := big.NewFloat(float64(args.CurrentValueInQuarks)).SetPrec(defaultCurvePrec)
	scaledCurrentValue := new(big.Float).Quo(unscaledCurrentValue, scale)

	scale = big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	scaledValue := DefaultExponentialCurve().ValueFromSellingTokens(scaledCurrentValue, scaledSellAmount)
	unscaledValue := new(big.Float).Mul(scaledValue, scale)

	scale = big.NewFloat(math.Pow10(int(args.ValueMintDecimals))).SetPrec(defaultCurvePrec)
	feePctValue := new(big.Float).SetPrec(defaultCurvePrec).Quo(big.NewFloat(float64(args.SellFeeBps)), big.NewFloat(10000))
	scaledFees := new(big.Float).Mul(scaledValue, feePctValue)
	unscaledFees := new(big.Float).Mul(scaledFees, scale)

	value, _ := unscaledValue.Int64()
	fees, _ := unscaledFees.Int64()
	return uint64(value - fees), uint64(fees)
}
