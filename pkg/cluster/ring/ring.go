package ring

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/spaolacci/murmur3"
)

// HashRing is a token ring that can be used for Consistent Hashing.
//
// Nodes place claims along a TokenRing in a distributed manner (rather than consecutively).
// This allows for less shuffling during claim changes in the ring. Construction of the ring
// is deterministic, which allows for multiple processes to have an (eventually) consistent
// view of the ring.
//
// Users should select a high number of claims for nodes. This is due to the fact that the
// underlying hash functions aren't 100% uniform. Selecting a larger claim value helps
// compensate for this fact. As the set of nodes increase, the number of claims per node
// can decrease (in theory).
type HashRing struct {
	hashFn func([]byte) uint64

	mu          sync.RWMutex
	ring        []uint64
	tokenOwners map[uint64]string
	nodeClaims  map[string]int
}

func NewHashRing() *HashRing {
	return NewRingFromMap(nil)
}

func NewRingFromMap(nodeClaims map[string]int) *HashRing {
	ring := &HashRing{
		hashFn:      murmur3.Sum64,
		ring:        make([]uint64, 0),
		tokenOwners: make(map[uint64]string),
		nodeClaims:  make(map[string]int),
	}
	for node, claims := range nodeClaims {
		_ = ring.SetClaims(node, claims)
	}
	return ring
}

// SetClaims sets the number of claims for the specified node.
//
// It only returns an error if an empty node or negative target is provided.
func (r *HashRing) SetClaims(node string, target int) error {
	if node == "" {
		return fmt.Errorf("node is empty")
	}
	if target < 0 {
		return fmt.Errorf("invalid number of target: %d", target)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.nodeClaims[node]

	if target == current {
		return nil
	}

	if target > current {
		for i := current; i < target; i++ {
			hash := r.hashFn([]byte(node + strconv.Itoa(i)))

			// Collisions are possible (although hopefully rare).
			//
			// To ensure deterministic behaviour for all members who compute the same ring, we
			// set the descendant that on collision, the 'node' with the lexicographically 'smaller'
			// value to be the owner.
			//
			// This has the side effect that on collision, there will be a slight skew towards one of
			// the nodes. However, this should be negligible as we generally use a high number of claims
			// (compared to the number of nodes) to try and achieve even distribution anyway.
			if tokenOwner, exists := r.tokenOwners[hash]; exists {
				if strings.Compare(tokenOwner, node) > 0 {
					continue
				}
			}

			r.ring = append(r.ring, hash)
			r.tokenOwners[hash] = node
		}

		slices.SortFunc(r.ring, func(a, b uint64) int {
			return cmp.Compare(a, b)
		})
	} else {
		for i := current - 1; i >= target; i-- {
			hash := r.hashFn([]byte(node + strconv.Itoa(i)))
			idx, found := sort.Find(len(r.ring), func(i int) int {
				return cmp.Compare(hash, r.ring[i]) // maybe this is weird...but shouldn't be
			})
			if !found {
				return fmt.Errorf("invalid ring state: claim %d does not exist", hash)
			}

			r.ring = slices.Delete(r.ring, idx, idx+1)
			delete(r.tokenOwners, hash)
		}
	}

	if target == 0 {
		delete(r.nodeClaims, node)
	} else {
		r.nodeClaims[node] = target
	}

	return nil
}

func (r *HashRing) GetNode(key []byte) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return ""
	}

	hash := r.hashFn([]byte(key))
	i := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i] >= hash
	})

	// If we didn't find a ring entry, wrap back around the ring.
	if i == len(r.ring) {
		i = 0
	}

	return r.tokenOwners[r.ring[i]]
}

func (r *HashRing) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.ring)
}
