package currencycreator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEstimateCurrentPrice(t *testing.T) {
	fmt.Println(EstimateCurrentPrice(0).Text('f', DefaultCurveDecimals))
	fmt.Println(EstimateCurrentPrice(DefaultMintMaxQuarkSupply).Text('f', DefaultCurveDecimals))
}

func TestEstimatValueExchange(t *testing.T) {
	quarks := EstimateValueExchange(&EstimateValueExchangeArgs{
		ValueInQuarks:        10000000,   // $10
		CurrentValueInQuarks: 1000000000, // $1000
		ValueMintDecimals:    6,
	})

	fmt.Printf("%d quarks\n", quarks)
}

func TestEstimateBuy(t *testing.T) {
	received, fees := EstimateBuy(&EstimateBuyArgs{
		BuyAmountInQuarks:     100000000,        // $100
		CurrentSupplyInQuarks: 7179502000000000, // 717,950.2 tokens
		ValueMintDecimals:     6,
		BuyFeeBps:             0, //0%
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateBuy(&EstimateBuyArgs{
		BuyAmountInQuarks:     100000000,        // $100
		CurrentSupplyInQuarks: 7179502000000000, // 717,950.2 tokens
		ValueMintDecimals:     6,
		BuyFeeBps:             100, // 1%
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}

func TestEstimateSell(t *testing.T) {
	received, fees := EstimateSell(&EstimateSellArgs{
		SellAmountInQuarks:   2651496281136, // 265.1496281136 tokens
		CurrentValueInQuarks: 10100000000,   // $10100
		ValueMintDecimals:    6,
		SellFeeBps:           0, // 0%
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)

	received, fees = EstimateSell(&EstimateSellArgs{
		SellAmountInQuarks:   2651496281136, // 265.1496281136 tokens
		CurrentValueInQuarks: 10100000000,   // $10100
		ValueMintDecimals:    6,
		SellFeeBps:           100, // 1%
	})
	fmt.Printf("%d total, %d received, %d fees\n", received+fees, received, fees)
}

func TestEstimates_CsvTable(t *testing.T) {
	startValue := uint64(10000)          // $0.01
	endValue := uint64(1000000000000000) // $1T

	fmt.Println("value locked,total circulating supply,payment value,payment quarks,sell value,new circulating supply")
	for valueLocked := startValue; valueLocked <= endValue; valueLocked *= 10 {
		totalCirculatingSupply, _ := EstimateBuy(&EstimateBuyArgs{
			BuyAmountInQuarks:     valueLocked,
			CurrentSupplyInQuarks: 0,
			ValueMintDecimals:     6,
		})

		for paymentValue := startValue; paymentValue <= valueLocked; paymentValue *= 10 {
			paymenQuarks := EstimateValueExchange(&EstimateValueExchangeArgs{
				ValueInQuarks:        paymentValue,
				CurrentValueInQuarks: valueLocked,
				ValueMintDecimals:    6,
			})

			sellValue, _ := EstimateSell(&EstimateSellArgs{
				SellAmountInQuarks:   paymenQuarks,
				CurrentValueInQuarks: valueLocked,
				ValueMintDecimals:    6,
			})

			diff := int64(paymentValue) - int64(sellValue)
			require.True(t, diff >= -1 && diff <= 1)

			newCirculatingSupply, _ := EstimateBuy(&EstimateBuyArgs{
				BuyAmountInQuarks:     valueLocked - paymentValue,
				CurrentSupplyInQuarks: 0,
				ValueMintDecimals:     6,
			})

			diff = int64(totalCirculatingSupply) - int64(newCirculatingSupply) - int64(paymenQuarks)
			require.True(t, diff >= -1 && diff <= 1)

			fmt.Printf("%d,%d,%d,%d,%d,%d\n", valueLocked, totalCirculatingSupply, paymentValue, paymenQuarks, sellValue, newCirculatingSupply)
		}
	}
}
