package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeAllKinExchangeRates_HappyPath(t *testing.T) {
	kinRates := map[string]float64{
		"usd": 0.5,
		"cad": 1.0,
	}

	usdRates := map[string]float64{
		"usd": 1.0,
		"cad": 1.3,
		"eur": 1.0,
		"aud": 0.66,
	}

	rates, err := computeAllKinExchangeRates(kinRates, usdRates)
	require.NoError(t, err)

	assert.Equal(t, rates["usd"], 0.5)
	assert.Equal(t, rates["cad"], 1.0) // FX rate differs, but we always prefer the source kin rate
	assert.Equal(t, rates["eur"], 0.5)
	assert.Equal(t, rates["aud"], 0.33)
}

func TestComputeAllKinExchangeRates_UsdRateMissing(t *testing.T) {
	kinRates := map[string]float64{
		"cad": 1.0,
	}

	usdRates := map[string]float64{
		"usd": 1.0,
		"cad": 1.3,
		"eur": 1.0,
		"aud": 0.66,
	}

	_, err := computeAllKinExchangeRates(kinRates, usdRates)
	assert.Error(t, err)
}
