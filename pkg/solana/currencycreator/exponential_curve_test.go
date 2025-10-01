package currencycreator

import (
	"fmt"
	"math/big"
	"testing"
)

func TestCalculateCurveConstants(t *testing.T) {
	curve := DefaultExponentialCurve()

	// Check R'(0) with tolerance
	spot0 := curve.SpotPriceAtSupply(big.NewFloat(0))
	expectedStart := big.NewFloat(0.01)
	diff := new(big.Float).Sub(spot0, expectedStart)
	threshold, _ := new(big.Float).SetString("0.0000000001")
	if diff.Abs(diff).Cmp(threshold) > 0 {
		t.Errorf("Spot at 0: got %s, expected 0.01", spot0.Text('f', DefaultCurveDecimals))
	}

	// Check R'(21000000) with tolerance
	supplyEnd := big.NewFloat(21000000)
	spotEnd := curve.SpotPriceAtSupply(supplyEnd)
	expectedEnd := big.NewFloat(1000000)
	diff = new(big.Float).Sub(spotEnd, expectedEnd)
	threshold, _ = new(big.Float).SetString("0.0001")
	if diff.Abs(diff).Cmp(threshold) > 0 {
		t.Errorf("Spot at end: got %s, expected 1000000", spotEnd.Text('f', DefaultCurveDecimals))
	}
}

func TestGenerateCurveTable(t *testing.T) {
	curve := DefaultExponentialCurve()

	fmt.Println("|------|----------------|----------------------------------|----------------------------|")
	fmt.Println("| %    | S              | R(S)                             | R'(S)                      |")
	fmt.Println("|------|----------------|----------------------------------|----------------------------|")

	zero := big.NewFloat(0)
	buyAmount := big.NewFloat(210000)
	supply := new(big.Float).Copy(zero)

	for i := 0; i <= 100; i++ {
		cost := curve.CostToBuyTokens(zero, supply)
		spotPrice := curve.SpotPriceAtSupply(supply)

		fmt.Printf("| %3d%% | %14s | %32s | %26s |\n",
			i,
			supply.Text('f', 0),
			cost.Text('f', DefaultCurveDecimals),
			spotPrice.Text('f', DefaultCurveDecimals))

		supply = supply.Add(supply, buyAmount)
	}

	fmt.Println("|------|----------------|----------------------------------|----------------------------|")
}
