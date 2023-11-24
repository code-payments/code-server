package sync

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRing_Consistency(t *testing.T) {
	entries := make(map[string]interface{})
	for i := 0; i < 64; i++ {
		entries[fmt.Sprintf("entry%d", i)] = i
	}

	r := newRing(entries, 200)

	for i := 0; i < 256; i++ {
		key := []byte(fmt.Sprintf("key%d", i))
		val := r.shard(key)

		for j := 0; j < 256; j++ {
			assert.EqualValues(t, val, r.shard(key))
		}
	}
}

func TestRing_Distribution(t *testing.T) {
	entryCount := 5
	iterations := 500000
	marginOfError := 0.1
	expectedFrequency := iterations / entryCount

	entries := make(map[string]interface{})
	for i := 0; i < entryCount; i++ {
		entries[fmt.Sprintf("entry%d", i)] = i
	}

	r := newRing(entries, 200)

	hits := make(map[int]int)
	for i := 0; i < iterations; i++ {
		key := []byte(fmt.Sprintf("key%d", i))
		val := r.shard(key).(int)
		hits[val]++
	}

	assert.EqualValues(t, entryCount, len(hits))
	for _, hitCount := range hits {
		assert.True(t, math.Abs(float64(hitCount-expectedFrequency)) <= marginOfError*float64(expectedFrequency))
	}
}
