package currencycreator

import (
	"fmt"
	"testing"
)

func TestEstimateCurrentPrice(t *testing.T) {
	t.Skip()

	fmt.Println(EstimateCurrentPrice(0).Text('f', DefaultCurveDecimals))
	fmt.Println(EstimateCurrentPrice(DefaultMintMaxSupply).Text('f', DefaultCurveDecimals))
}

func TestEstimateBuyInUsdc(t *testing.T) {
	t.Skip()

	received, fees := EstimateBuyInUsdc(&EstimateBuyInUsdcArgs{
		BuyAmountInQuarks:     50_000_000,
		CurrentSupplyInQuarks: 0,
		BuyFeeBps:             0,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateBuyInUsdc(&EstimateBuyInUsdcArgs{
		BuyAmountInQuarks:     50_000_000,
		CurrentSupplyInQuarks: 4_989_067_263,
		BuyFeeBps:             100,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}

func TestEstimateSelInUsdc(t *testing.T) {
	t.Skip()

	received, fees := EstimateSellInUsdc(&EstimateSellInUsdcArgs{
		SellAmountInQuarks:   1_234_567_890,
		CurrentValueInQuarks: 50_000_000,
		SellFeeBps:           0,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateSellInUsdc(&EstimateSellInUsdcArgs{
		SellAmountInQuarks:   1_234_567_890,
		CurrentValueInQuarks: 50_000_000,
		SellFeeBps:           100,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}
