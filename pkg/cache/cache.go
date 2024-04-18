package cache

import (
	"log"
	"sync"
)

// Cache stores arbitrary data for fast retrieval
type Cache interface {
	SetVerbose(verbose bool)
	GetWeight() int
	GetBudget() int
	Insert(key string, value interface{}, weight int) (inserted bool)
	Retrieve(key string) (interface{}, bool)
	Clear()
}

// Cacher is something that has a cache
type Cacher interface {
	ClearCache()
	GetCache() Cache
}

type cacheNode struct {
	next   *cacheNode
	prev   *cacheNode
	key    string
	value  interface{}
	weight int
}

// Cache stores arbitrary data for fast retrieval
type cache struct {
	head    *cacheNode
	tail    *cacheNode
	lookup  map[string]*cacheNode
	weight  int
	budget  int
	verbose bool
	mutex   sync.Mutex
}

func NewCache(budget int) Cache {
	return &cache{lookup: make(map[string]*cacheNode), budget: budget}
}

// SetVerbose turns on verbose printing (warnings and stuff)
func (c *cache) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// GetWeight gets the "weight" of a cache
func (c *cache) GetWeight() int {
	return c.weight
}

// GetBudget gets the memory budget of a cache
func (c *cache) GetBudget() int {
	return c.budget
}

// Insert inserts an object into the cache
func (c *cache) Insert(key string, value interface{}, weight int) (inserted bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, found := c.lookup[key]; found {
		return false
	}

	node := &cacheNode{
		key:    key,
		value:  value,
		weight: weight,
		next:   c.head,
	}

	if c.head != nil {
		c.head.prev = node
	}

	c.head = node
	if c.tail == nil {
		c.tail = node
	}

	c.lookup[key] = node
	c.weight += node.weight

	for ; c.tail != nil && c.tail != c.head && c.weight > c.budget; c.tail = c.tail.prev {
		c.weight -= c.tail.weight
		c.tail.prev.next = nil

		if c.verbose {
			log.Printf(
				"warning -- cache is evicting %s (%d) for %s (%d); spare weight is now %d",
				c.tail.key,
				c.tail.weight,
				key,
				weight,
				c.budget-c.weight,
			)
		}

		delete(c.lookup, c.tail.key)
	}

	return true
}

// Retrieve gets an object out of the cache
func (c *cache) Retrieve(key string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	node, found := c.lookup[key]
	if !found {
		return nil, false
	}

	if node != c.head {
		if node.next != nil {
			node.next.prev = node.prev
		}

		if node.prev != nil {
			node.prev.next = node.next
		}

		if node == c.tail {
			c.tail = c.tail.prev
		}

		node.next = c.head
		node.prev = nil

		if c.head != nil {
			c.head.prev = node
		}

		c.head = node
	}

	return node.value, true
}

// Clear removes all cache entries
func (c *cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.head = nil
	c.tail = nil
	c.lookup = make(map[string]*cacheNode)
	c.weight = 0
}
