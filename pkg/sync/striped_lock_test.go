package sync

import (
	"fmt"
	"sync"
	base "sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripedLock_HappyPath(t *testing.T) {
	workerCount := 256
	operationCount := 100000

	l := NewStripedLock(4)

	var workerWg base.WaitGroup
	startChan := make(chan struct{}, 0)
	data := make([]int, workerCount)

	for i := 0; i < workerCount; i++ {
		workerWg.Add(1)

		go func(workerID int) {
			defer workerWg.Done()

			var opWg sync.WaitGroup
			key := []byte(fmt.Sprintf("worker%d", workerID))
			for j := 0; j < operationCount; j++ {
				opWg.Add(1)

				go func() {
					defer opWg.Done()

					select {
					case <-startChan:
					}

					mu := l.Get([]byte(key))
					mu.Lock()
					data[workerID]++
					mu.Unlock()
				}()
			}
			opWg.Wait()
		}(i)
	}

	close(startChan)
	workerWg.Wait()

	for _, val := range data {
		assert.EqualValues(t, operationCount, val)
	}
}
