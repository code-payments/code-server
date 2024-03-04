package cache

import (
	"errors"
	"log"
	"sync"
)

// Cache interface defines the operations for a generic cache system,
// allowing for setting verbosity, retrieving weight and budget constraints,
// and operations to insert, retrieve, and clear cached items.
type Cache interface {
	SetVerbose(verbose bool)           // Control logging verbosity
	GetWeight() int                    // Get the current weight of the cache
	GetBudget() int                    // Get the weight budget of the cache
	Insert(key string, value interface{}, weight int) error // Insert a new item
	Retrieve(key string) (interface{}, bool) // Retrieve an item by key
	Clear()                            // Clear all items from the cache
}

// cacheNode represents a node in the doubly-linked list, storing each
// cache item along with its key, value, and weight.
type cacheNode struct {
	next   *cacheNode
	prev   *cacheNode
	key    string
	value  interface{}
	weight int
}

// cache struct implements the Cache interface, providing a mechanism
// to store and retrieve arbitrary data with weight constraints. It uses
// a map for quick lookup and a doubly-linked list to manage item order.
type cache struct {
	head    *cacheNode                 // Pointer to the first node
	tail    *cacheNode                 // Pointer to the last node
	lookup  map[string]*cacheNode      // Map for quick node lookup by key
	weight  int                        // Current weight of the cache
	budget  int                        // Maximum allowed weight
	verbose bool                       // Verbose logging flag
	mutex   sync.Mutex                 // Mutex to protect concurrent access
}

// NewCache initializes and returns a new cache with a given weight budget.
func NewCache(budget int) Cache {
	return &cache{
		lookup: make(map[string]*cacheNode),
		budget: budget,
	}
}

// SetVerbose sets the verbosity of cache operation logging.
func (c *cache) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// GetWeight returns the current total weight of items in the cache.
func (c *cache) GetWeight() int {
	return c.weight
}

// GetBudget returns the weight budget of the cache.
func (c *cache) GetBudget() int {
	return c.budget
}

// Insert adds a new item to the cache. If the key already exists, it returns an error.
// It also ensures that the cache does not exceed its weight budget by evicting
// the least recently used items as necessary.
func (c *cache) Insert(key string, value interface{}, weight int) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if the key already exists
	if _, found := c.lookup[key]; found {
		return errors.New("key already exists in cache")
	}

	// Create a new node for the item
	node := &cacheNode{
		key:    key,
		value:  value,
		weight: weight,
		next:   c.head,
	}

	// Insert the new node at the beginning of the doubly-linked list
	if c.head != nil {
		c.head.prev = node
	}
	c.head = node
	if c.tail == nil {
		c.tail = node
	}

	// Add the node to the lookup map and update the cache's current weight
	c.lookup[key] = node
	c.weight += weight

	// Evict least recently used items if the cache exceeds its budget
	for c.weight > c.budget && c.tail != nil {
		evictNode := c.tail
		if c.tail.prev != nil {
			c.tail.prev.next = nil
		} else {
			c.head = nil // The cache will be empty after this node is evicted
		}
		c.tail = c.tail.prev
		c.weight -= evictNode.weight
		delete(c.lookup, evictNode.key)

		if c.verbose {
			// Log the eviction event
			log.Printf(
				"Cache eviction: Removed %s (Weight: %d); New spare weight: %d",
				evictNode.key, evictNode.weight, c.budget-c.weight,
			)
		}
	}

	return nil
}

// Retrieve fetches an item from the cache by its key, if it exists.
// It returns the item and a boolean indicating whether the key was found.
// The method also moves the retrieved item to the front of the list to
// indicate that it was recently used.
func (c *cache) Retrieve(key string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	node, found := c.lookup[key]
	if !found {
		return nil, false
	}

	// Move the node to the front of the list if it's not already the first node
	if node != c.head {
		// Adjust pointers to remove the node from its current position
		if node.next != nil {
			node.next.prev = node.prev
		}
		if node.prev != nil {
			node.prev.next = node.next
		}
		if node == c.tail {
			c.tail = node.prev
		}

		// Insert the node at the beginning of the list
		node.next = c.head
		node.prev = nil
		if c.head != nil {
			c.head.prev = node
		}
		c.head = node
	}

	return node.value, true
}

// Clear removes all items from the cache, resetting it to an empty state.
func (c *cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.head = nil
	c.tail = nil
	c.lookup = make(map[string]*cacheNode)
	c.weight = 0
}
