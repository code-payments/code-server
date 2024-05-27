package ring

import (
	"math"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	randomKeyProvider = func() string {
		return strconv.Itoa(rand.Int())
	}
)

func TestHashRingRandomBalancing(t *testing.T) {
	require := require.New(t)

	testCases := []struct {
		claims       map[string]int
		expectedFreq map[string]float64
		expectedErr  bool
		KeyProvider  func() string
	}{
		{
			claims: map[string]int{
				"nodeA": 50,
				"nodeB": 50,
				"nodeC": 100,
			},
			expectedFreq: map[string]float64{
				"nodeA": 0.25,
				"nodeB": 0.25,
				"nodeC": 0.5,
			},
			expectedErr: false,
			KeyProvider: randomKeyProvider,
		},
		// One node no weight
		{
			claims: map[string]int{
				"nodeA": 0,
				"nodeB": 100,
				"nodeC": 100,
			},
			expectedFreq: map[string]float64{
				"nodeA": 0,
				"nodeB": 0.5,
				"nodeC": 0.5,
			},
			expectedErr: false,
			KeyProvider: randomKeyProvider,
		},
		// No nodeClaims in ring; expect error
		{
			claims: map[string]int{
				"nodeA": 0,
				"nodeB": 0,
				"nodeC": 0,
			},
			expectedFreq: nil,
			expectedErr:  true,
			KeyProvider:  randomKeyProvider,
		},
	}
	for _, tc := range testCases {
		totalClaims := 0
		for _, v := range tc.claims {
			totalClaims += v
		}

		ring := NewRingFromMap(tc.claims)
		require.Equal(totalClaims, ring.Size())

		if tc.expectedErr {
			node := ring.GetNode([]byte(tc.KeyProvider()))
			require.Equal("", node)
			continue
		}

		verifyFrequency(require, ring, tc.KeyProvider, tc.expectedFreq)
	}
}

func TestHashRingClaimUpdate(t *testing.T) {
	require := require.New(t)

	ring := NewRingFromMap(map[string]int{
		"nodeA": 0,
		"nodeB": 100,
		"nodeC": 100,
	})
	require.Equal(200, ring.Size())

	verifyFrequency(require, ring, randomKeyProvider, map[string]float64{
		"nodeA": 0,
		"nodeB": 0.5,
		"nodeC": 0.5,
	})

	// Increase nodeA nodeClaims
	require.NoError(ring.SetClaims("nodeA", 100))
	require.Equal(300, ring.Size())

	verifyFrequency(require, ring, randomKeyProvider, map[string]float64{
		"nodeA": 0.333,
		"nodeB": 0.333,
		"nodeC": 0.333,
	})

	// Decrease nodeB,C nodeClaims
	require.NoError(ring.SetClaims("nodeB", 50))
	require.NoError(ring.SetClaims("nodeC", 50))
	require.Equal(200, ring.Size())

	verifyFrequency(require, ring, randomKeyProvider, map[string]float64{
		"nodeA": 0.5,
		"nodeB": 0.25,
		"nodeC": 0.25,
	})

	// Remove nodeB nodeClaims
	require.NoError(ring.SetClaims("nodeB", 0))
	require.Equal(150, ring.Size())

	verifyFrequency(require, ring, randomKeyProvider, map[string]float64{
		"nodeA": 0.666,
		"nodeB": 0,
		"nodeC": 0.333,
	})
}

func TestHashRingConsistency(t *testing.T) {
	require := require.New(t)

	ring := NewRingFromMap(map[string]int{
		"nodeA": 100,
		"nodeB": 100,
		"nodeC": 100,
	})
	require.Equal(300, ring.Size())

	// Test two constant hash keys; they should consistently be
	// mapped to a single node regardless of weight
	verifyFrequency(require, ring, func() string { return "testKey1" }, map[string]float64{
		"nodeA": 1,
		"nodeB": 0,
		"nodeC": 0,
	})
	verifyFrequency(require, ring, func() string { return "testKey2" }, map[string]float64{
		"nodeA": 1,
		"nodeB": 0,
		"nodeC": 0,
	})

	// Set nodeC weight to 0
	require.NoError(ring.SetClaims("nodeC", 0))

	// Key that mapped to nodeC now re-mapped
	verifyFrequency(require, ring, func() string { return "testKey1" }, map[string]float64{
		"nodeA": 1,
		"nodeB": 0,
		"nodeC": 0,
	})
}

func verifyFrequency(require *require.Assertions, ring *HashRing, keyProvider func() string, expectedFreq map[string]float64) {
	iterations := 500
	actualFreq := map[string]float64{}
	for i := 0; i < iterations; i++ {
		node := ring.GetNode([]byte(keyProvider()))
		require.NotNil(node)
		if _, ok := actualFreq[node]; !ok {
			actualFreq[node] = 0
		}
		actualFreq[node]++
	}

	for ep, freq := range actualFreq {
		actualFreq[ep] = freq / float64(iterations)
	}

	for ep, expected := range expectedFreq {
		actual, hasBeenPicked := actualFreq[ep]
		if !hasBeenPicked {
			require.Equal(float64(0), expected)
		} else {
			// Allow a delta of 10% to avoid test flakiness
			require.True(math.Abs(actual-expected) <= 0.10, "frequency doesn't match: want: %v got: %v", expectedFreq, actualFreq)
		}
	}
}
