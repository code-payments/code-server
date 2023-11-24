package sync

import (
	"fmt"
	base "sync"
)

const (
	hashEntriesPerLock = 200
)

// StripedLock is a partitioned locking mechanism that consistently maps a key
// space to a set of locks. This provides concurrent data access while also
// limiting the total memory footprint.
type StripedLock struct {
	locks    []base.RWMutex
	hashRing *ring
}

// NewStripedLock returns a new StripedLock with a static number of stripes.
func NewStripedLock(stripes uint) *StripedLock {
	return newStripedLockGroup(stripes, 1)[0]
}

// newStripedLockGroup returns a new group of StripedLocks which share a common
// hash ring. This should be used in cases where multiple striped locks are
// needed which share the same sharding domain in order to avoid allocating
// entire hash rings for each.
//
// todo: Add tests, then expose publicly
func newStripedLockGroup(stripes, groupSize uint) []*StripedLock {
	stripedLocks := make([]*StripedLock, 0, groupSize)

	ringEntries := make(map[string]interface{})
	for i := 0; i < int(stripes); i++ {
		ringEntries[fmt.Sprintf("lock%d", i)] = i
	}

	ring := newRing(ringEntries, hashEntriesPerLock)

	for i := 0; i < int(groupSize); i++ {
		stripedLocks = append(stripedLocks, &StripedLock{
			locks:    make([]base.RWMutex, stripes),
			hashRing: ring,
		})
	}

	return stripedLocks
}

// Get gets the lock for a key
func (l *StripedLock) Get(key []byte) *base.RWMutex {
	sharded := l.hashRing.shard(key).(int)
	return &l.locks[sharded]
}
