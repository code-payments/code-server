package sync

import (
	"fmt"
	"sync"
)

const (
	hashEntriesPerChannel = 200
)

// StripedChannel is a partitioned channel that consistently maps a key space
// to a set of channels.
type StripedChannel struct {
	channels  []chan interface{}
	hashRing  *ring
	closeFunc sync.Once
}

// NewStripedChannel returns a new StripedChannel with a static number of
// channels.
func NewStripedChannel(count, queueSize uint) *StripedChannel {
	channels := make([]chan interface{}, count)

	ringEntries := make(map[string]interface{})
	for i := range channels {
		channels[i] = make(chan interface{}, queueSize)
		ringEntries[fmt.Sprintf("chan%d", i)] = i
	}

	return &StripedChannel{
		channels: channels,
		hashRing: newRing(ringEntries, hashEntriesPerChannel),
	}
}

// GetChannels returns the set of all receiver channels.
func (c *StripedChannel) GetChannels() []<-chan interface{} {
	receivers := make([]<-chan interface{}, len(c.channels))
	for i, channel := range c.channels {
		receivers[i] = channel
	}
	return receivers
}

// Send sends the value to the channel that maps to the key. It is non-blocking
// and returns whether the value was put on the channel.
func (c *StripedChannel) Send(key []byte, value interface{}) bool {
	sharded := c.hashRing.shard(key).(int)
	select {
	case c.channels[sharded] <- value:
	default:
		return false
	}
	return true
}

// BlockingSend is a blocking variation of Send.
func (c *StripedChannel) BlockingSend(key []byte, value interface{}) {
	sharded := c.hashRing.shard(key).(int)
	c.channels[sharded] <- value
}

// Close closes all underlying channels.
func (c *StripedChannel) Close() {
	c.closeFunc.Do(func() {
		for _, channel := range c.channels {
			close(channel)
		}
	})
}
