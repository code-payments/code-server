package currencycreator

import (
	"math"
	"math/big"
)

type EstimateCurrentPriceArgs struct {
	Curve                 *ExponentialCurve
	CurrentSupplyInQuarks uint64
	MintDecimals          uint8
}

func EstimateCurrentPrice(args *EstimateCurrentPriceArgs) *big.Float {
	scale := big.NewFloat(math.Pow10(int(args.MintDecimals))).SetPrec(defaultExponentialCurvePrec)
	unscaledCurrentSupply := big.NewFloat(float64(args.CurrentSupplyInQuarks)).SetPrec(defaultExponentialCurvePrec)
	scaledCurrentSupply := new(big.Float).Quo(unscaledCurrentSupply, scale)
	return args.Curve.SpotPriceAtSupply(scaledCurrentSupply)
}

type EstimateBuyArgs struct {
	BuyAmountInQuarks     uint64
	Curve                 *ExponentialCurve
	CurrentSupplyInQuarks uint64
	BuyFeeBps             uint32
	TargetMintDecimals    uint8
	BaseMintDecimals      uint8
}

func EstimateBuy(args *EstimateBuyArgs) (uint64, uint64) {
	scale := big.NewFloat(math.Pow10(int(args.BaseMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	unscaledBuyAmount := big.NewFloat(float64(args.BuyAmountInQuarks)).SetPrec(defaultExponentialCurvePrec)
	scaledBuyAmount := new(big.Float).Quo(unscaledBuyAmount, scale)

	scale = big.NewFloat(math.Pow10(int(args.TargetMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	unscaledCurrentSupply := big.NewFloat(float64(args.CurrentSupplyInQuarks)).SetPrec(defaultExponentialCurvePrec)
	scaledCurrentSupply := new(big.Float).Quo(unscaledCurrentSupply, scale)

	scale = big.NewFloat(math.Pow10(int(args.TargetMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	scaledTotalValue := args.Curve.ValueToTokens(scaledCurrentSupply, scaledBuyAmount)
	unscaledTotalValue := new(big.Float).Mul(scaledTotalValue, scale)

	feePctValue := new(big.Float).SetPrec(defaultExponentialCurvePrec).Quo(big.NewFloat(float64(args.BuyFeeBps)), big.NewFloat(10000))
	scaledFees := new(big.Float).Mul(scaledTotalValue, feePctValue)
	unscaledFees := new(big.Float).Mul(scaledFees, scale)

	total, _ := unscaledTotalValue.Int64()
	fees, _ := unscaledFees.Int64()
	return uint64(total - fees), uint64(fees)
}

type EstimateSaleArgs struct {
	SellAmountInQuarks    uint64
	Curve                 *ExponentialCurve
	CurrentSupplyInQuarks uint64
	SellFeeBps            uint32
	TargetMintDecimals    uint8
	BaseMintDecimals      uint8
}

func EstimateSale(args *EstimateSaleArgs) (uint64, uint64) {
	scale := big.NewFloat(math.Pow10(int(args.TargetMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	unscaledSellAmount := big.NewFloat(float64(args.SellAmountInQuarks)).SetPrec(defaultExponentialCurvePrec)
	scaledSellAmount := new(big.Float).Quo(unscaledSellAmount, scale)

	scale = big.NewFloat(math.Pow10(int(args.TargetMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	unscaledCurrentSupply := big.NewFloat(float64(args.CurrentSupplyInQuarks)).SetPrec(defaultExponentialCurvePrec)
	scaledCurrentSupply := new(big.Float).Quo(unscaledCurrentSupply, scale)

	scale = big.NewFloat(math.Pow10(int(args.BaseMintDecimals))).SetPrec(defaultExponentialCurvePrec)
	scaledTotalValue := args.Curve.TokensToValue(scaledCurrentSupply, scaledSellAmount)
	unscaledTotalValue := new(big.Float).Mul(scaledTotalValue, scale)

	feePctValue := new(big.Float).SetPrec(defaultExponentialCurvePrec).Quo(big.NewFloat(float64(args.SellFeeBps)), big.NewFloat(10000))
	scaledFees := new(big.Float).Mul(scaledTotalValue, feePctValue)
	unscaledFees := new(big.Float).Mul(scaledFees, scale)

	total, _ := unscaledTotalValue.Int64()
	fees, _ := unscaledFees.Int64()
	return uint64(total - fees), uint64(fees)
}
