package cache

import (
	"sync"
	"testing"
)

// TestCacheInsert verifies that inserting an item into the cache works correctly
// and does not produce an error when done within the cache's budget constraints.
func TestCacheInsert(t *testing.T) {
	cache := NewCache(10) // Increased budget for a more realistic test scenario
	insertError := cache.Insert("A", "valueA", 1)

	if insertError != nil {
		t.Fatalf("Cache insert resulted in unexpected error: %s", insertError)
	}
}

// TestCacheInsertWithinBudget checks if items can be inserted without exceeding
// the cache's weight budget, ensuring the cache correctly manages its constraints.
func TestCacheInsertWithinBudget(t *testing.T) {
	cache := NewCache(3) // Adjusted budget to allow for multiple items
	// Insert multiple items within the budget
	if err := cache.Insert("A", "valueA", 1); err != nil {
		t.Fatalf("Cache insert resulted in unexpected error: %s", err)
	}
	if err := cache.Insert("B", "valueB", 1); err != nil {
		t.Fatalf("Cache insert resulted in unexpected error: %s", err)
	}
	if err := cache.Insert("C", "valueC", 1); err != nil {
		t.Fatalf("Cache insert resulted in unexpected error: %s", err)
	}
	// Verify the total weight matches the inserted items' weights
	if weight := cache.GetWeight(); weight != 3 {
		t.Fatalf("Expected cache weight to be 3, got %d", weight)
	}
}

// TestCacheInsertUpdatesWeight verifies that the cache correctly updates its
// total weight as items are inserted and ensures that eviction happens correctly
// when the budget is exceeded, maintaining the cache's weight within its budget.
func TestCacheInsertUpdatesWeight(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("A", "valueA", 1)
	_ = cache.Insert("B", "valueB", 1)
	// Inserting an additional item should cause eviction due to budget exceedance
	_ = cache.Insert("C", "valueC", 1)

	if weight := cache.GetWeight(); weight != 2 {
		t.Fatal("Cache with budget 2 did not correctly update weight after evictions")
	}
}

// TestCacheInsertDuplicateRejected ensures that attempting to insert a duplicate
// key results in an error, as each key in the cache should be unique.
func TestCacheInsertDuplicateRejected(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("dupe", "valueDupe", 1)
	dupeError := cache.Insert("dupe", "valueDupe", 1)

	if dupeError == nil {
		t.Fatal("Cache insert of duplicate key did not result in any error")
	}
}

// TestCacheInsertEvictsLeastRecentlyUsed verifies that the cache evicts the least
// recently used item when exceeding its weight budget, ensuring that the cache's
// eviction policy is correctly implemented.
func TestCacheInsertEvictsLeastRecentlyUsed(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("evicted", "valueEvicted", 1)
	_ = cache.Insert("A", "valueA", 1)
	_ = cache.Insert("B", "valueB", 1)

	_, foundEvicted := cache.Retrieve("evicted")
	if foundEvicted {
		t.Fatal("Cache did not evict the least recently used item upon exceeding budget")
	}

	// Verify that other items are still present
	_, foundA := cache.Retrieve("A")
	_, foundB := cache.Retrieve("B")
	if !foundA || !foundB {
		t.Fatal("Cache eviction policy incorrectly removed items")
	}
}

// TestCacheInsertEvictsLeastRecentlyRetrieved tests the cache's behavior to
// ensure that it correctly identifies and evicts the least recently retrieved
// item when necessary, following its eviction policy.
func TestCacheInsertEvictsLeastRecentlyRetrieved(t *testing.T) {
	cache := NewCache(2)
	_ = cache.Insert("A", "valueA", 1)
	_ = cache.Insert("B", "valueB", 1)
	// Accessing "A" makes "B" the least recently used item
	cache.Retrieve("A")
	// Inserting a new item causes eviction due to budget exceedance
	_ = cache.Insert("C", "valueC", 1)

	_, foundB := cache.Retrieve("B")
	if foundB {
		t.Fatal("Cache did not evict the least recently used item after retrieval")
	}
}

// TestClear verifies that calling Clear() on the cache removes all items,
// ensuring that no retrievable items remain in the cache after it is cleared.
func TestClear(t *testing.T) {
	cache := NewCache(1)
	_ = cache.Insert("cleared", "valueCleared", 1)
	cache.Clear()

	_, found := cache.Retrieve("cleared")
	if found {
		t.Fatal("Cache did not remove all items upon calling Clear()")
	}
}

// Optional: Test for concurrency could be added to ensure the cache behaves correctly under concurrent access