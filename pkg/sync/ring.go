package sync

import (
	"encoding/binary"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/spaolacci/murmur3"
)

// ring is a consistent hash ring
//
// todo: extract into it's own package if there becomes more uses with more
//       options like hash func
type ring struct {
	hashRing *treemap.Map

	// minEntryValue is used to cache the value of min entry in hashRing.
	// Using treemap.Map.Min() is O(log n).
	minEntryValue interface{}
}

// newRing returns a new consistent hash ring with the set of entries that have
// replicationFactor entries in the ring
func newRing(entries map[string]interface{}, replicationFactor uint) *ring {
	hashRing := treemap.NewWith(utils.Int64Comparator)
	for k, v := range entries {
		keyHash, _ := murmur3.Sum128([]byte(k))
		keyHashBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(keyHashBytes, keyHash)
		for i := 0; i < int(replicationFactor); i++ {
			hasher := murmur3.New128()
			hasher.Write(keyHashBytes)
			indexBytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(indexBytes, uint32(i))
			hasher.Write(indexBytes)
			hash, _ := hasher.Sum128()
			hashRing.Put(int64(hash), v)
		}
	}

	_, minEntryValue := hashRing.Min()

	return &ring{
		hashRing:      hashRing,
		minEntryValue: minEntryValue,
	}
}

// shard consistently hashes the key and returns the sharded entry value
func (r *ring) shard(key []byte) interface{} {
	hasher := murmur3.New128()
	hasher.Write(key)
	raw, _ := hasher.Sum128()
	hash := int64(raw)
	_, shard := r.hashRing.Ceiling(hash)
	if shard != nil {
		return shard
	}
	return r.minEntryValue
}
