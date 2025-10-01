package currencycreator

import (
	"fmt"
	"testing"
)

func TestEstimateCurrentPrice(t *testing.T) {
	fmt.Println(EstimateCurrentPrice(0).Text('f', DefaultCurveDecimals))
	fmt.Println(EstimateCurrentPrice(DefaultMintMaxQuarkSupply).Text('f', DefaultCurveDecimals))
}

func TestEstimatValueExchange(t *testing.T) {
	quarks := EstimateValueExchange(&EstimateValueExchangeArgs{
		ValueInQuarks:         5_000_000,
		CurrentSupplyInQuarks: 7_232_649_000_000_000,
		ValueMintDecimals:     6,
	})

	fmt.Printf("%d quarks\n", quarks)
}

func TestEstimateBuy(t *testing.T) {
	received, fees := EstimateBuy(&EstimateBuyArgs{
		BuyAmountInQuarks:     50_000_000,
		CurrentSupplyInQuarks: 0,
		ValueMintDecimals:     6,
		BuyFeeBps:             0,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateBuy(&EstimateBuyArgs{
		BuyAmountInQuarks: 50_000_000,

		CurrentSupplyInQuarks: 4_989_067_263,
		ValueMintDecimals:     6,
		BuyFeeBps:             100,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}

func TestEstimateSell(t *testing.T) {
	received, fees := EstimateSell(&EstimateSellArgs{
		SellAmountInQuarks:   12_345_678_900_000,
		CurrentValueInQuarks: 50_000_000,
		ValueMintDecimals:    6,
		SellFeeBps:           0,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateSell(&EstimateSellArgs{
		SellAmountInQuarks:   12_345_678_900_000,
		CurrentValueInQuarks: 50_000_000,
		ValueMintDecimals:    6,
		SellFeeBps:           100,
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}
