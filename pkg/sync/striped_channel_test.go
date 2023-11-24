package sync

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripedChannel_HappyPath(t *testing.T) {
	c := NewStripedChannel(32, 256)

	channels := c.GetChannels()
	require.Equal(t, 32, len(channels))

	results := make([]map[int]int, len(channels))

	var wg sync.WaitGroup
	worker := func(id int, c <-chan interface{}) {
		defer func() {
			wg.Done()
		}()

		for {
			select {
			case val, ok := <-c:
				if !ok {
					return
				}

				results[id][val.(int)]++
			}
		}
	}

	for i, channel := range channels {
		wg.Add(1)
		results[i] = make(map[int]int)
		go worker(i, channel)
	}

	for i := 0; i < 256; i++ {
		for j := 0; j < 10; j++ {
			assert.True(t, c.Send([]byte{byte(i)}, i))
		}
	}

	c.Close()

	wg.Wait()

	aggregated := make(map[int]int)
	for _, result := range results {
		for k, v := range result {
			aggregated[k] = v
		}
	}

	assert.Equal(t, 256, len(aggregated))
	for _, v := range aggregated {
		assert.Equal(t, 10, v)
	}
}

func TestStripedChannel_FullQueue(t *testing.T) {
	c := NewStripedChannel(32, 256)

	for i := 0; i < 256; i++ {
		assert.True(t, c.Send([]byte{byte(1)}, 1))
	}

	assert.False(t, c.Send([]byte{byte(1)}, 1))
	assert.True(t, c.Send([]byte{byte(2)}, 2))
}
