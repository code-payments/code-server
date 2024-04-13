package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCacheInsert(t *testing.T) {
	cache := NewCache(1)
	require.True(t, cache.Insert("A", "", 1))
}

func TestCacheInsertWithinBudget(t *testing.T) {
	cache := NewCache(1)
	require.True(t, cache.Insert("A", "", 2))
}

func TestCacheInsertUpdatesWeight(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("A", "", 1)
	_ = cache.Insert("B", "", 1)
	_ = cache.Insert("budget_exceeded", "", 1)

	require.Equal(t, 2, cache.GetWeight())
}

func TestCacheInsertDuplicateRejected(t *testing.T) {
	cache := NewCache(2)
	require.True(t, cache.Insert("dupe", "", 1))
	require.False(t, cache.Insert("dupe", "", 1))
}

func TestCacheInsertEvictsLeastRecentlyUsed(t *testing.T) {
	cache := NewCache(2)
	// with a budget of 2, inserting 3 keys should evict the last
	_ = cache.Insert("evicted", "", 1)
	_ = cache.Insert("A", "", 1)
	_ = cache.Insert("B", "", 1)

	_, foundEvicted := cache.Retrieve("evicted")
	require.False(t, foundEvicted)

	// double check that only 1 one was evicted and not any extra
	_, foundA := cache.Retrieve("A")
	require.True(t, foundA)
	_, foundB := cache.Retrieve("B")
	require.True(t, foundB)
}

func TestCacheInsertEvictsLeastRecentlyRetrieved(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("A", "", 1)
	_ = cache.Insert("evicted", "", 1)

	// retrieve the oldest node, promoting it head, so it is not evicted
	cache.Retrieve("A")

	// insert once more, exceeding weight capacity
	_ = cache.Insert("B", "", 1)
	// now the least recently used key should be evicted
	_, foundEvicted := cache.Retrieve("evicted")
	require.False(t, foundEvicted)
}

func TestClear(t *testing.T) {
	cache := NewCache(1)
	_ = cache.Insert("cleared", "", 1)
	cache.Clear()
	_, found := cache.Retrieve("cleared")
	require.False(t, found)
}
