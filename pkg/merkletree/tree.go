package merkletree

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

type Hash []byte
type Seed []byte
type Leaf []byte

const (
	// todo: 64 levels causes an overflow issue
	MaxLevels = 63
)

var (
	ErrMerkleTreeFull    = errors.New("merkle tree is full")
	ErrInvalidLevelCount = errors.New("level count is invalid")
	ErrLeafNotFound      = errors.New("leaf not found")
)

// Reference in-memory implementation only. It's not terribly performant, but can
// be used to validate other more optimized implementations.
type MerkleTree struct {
	levels         uint8
	nextIndex      uint64
	root           Hash
	leaves         []Hash
	filledSubtrees []Hash
	zeroValues     []Hash
}

func New(levels uint8, seeds []Seed) (*MerkleTree, error) {
	if levels < 1 {
		return nil, ErrInvalidLevelCount
	}
	if levels > MaxLevels {
		return nil, ErrInvalidLevelCount
	}

	zeroValues := calculateZeroValues(levels, seeds)
	filledSubtrees := calculateZeroValues(levels, seeds)

	return &MerkleTree{
		levels:         levels,
		nextIndex:      0,
		root:           zeroValues[len(zeroValues)-1],
		filledSubtrees: filledSubtrees,
		zeroValues:     zeroValues,
	}, nil
}

func (t *MerkleTree) AddLeaf(leaf Leaf) error {
	if t.nextIndex >= uint64(math.Pow(2, float64(t.levels))) {
		return ErrMerkleTreeFull
	}

	currentIndex := t.nextIndex
	currentLevelHash := hash(leaf)

	t.leaves = append(t.leaves, currentLevelHash)

	var left, right Hash
	for i := 0; i < int(t.levels); i++ {
		if currentIndex%2 == 0 {
			left = currentLevelHash
			right = t.zeroValues[i]
			t.filledSubtrees[i] = currentLevelHash
		} else {
			left = t.filledSubtrees[i]
			right = currentLevelHash
		}

		currentLevelHash = hashLeftRight(left, right)
		currentIndex = currentIndex / 2
	}

	t.root = currentLevelHash
	t.nextIndex++

	return nil
}

func (t *MerkleTree) GetRoot() Hash {
	var cpy Hash
	return append(cpy, t.root...)
}

func (t *MerkleTree) GetLeafHash(leaf Leaf) Hash {
	return hash(leaf)
}

func (t *MerkleTree) GetIndexForLeaf(leaf Leaf) (int, error) {
	hashed := hash(leaf)
	for i := 0; i < len(t.leaves); i++ {
		if bytes.Equal(hashed, t.leaves[i]) {
			return i, nil
		}
	}

	return 0, ErrLeafNotFound
}

func (t *MerkleTree) GetLeafCount() uint64 {
	return uint64(len(t.leaves))
}

func (t *MerkleTree) GetExpectedHashFromPair(h1, h2 Hash) Hash {
	return hashLeftRight(h1, h2)
}

func (t *MerkleTree) GetZeroValues() []Hash {
	cpy := make([]Hash, len(t.zeroValues))
	for i, zeroValue := range t.zeroValues {
		cpy[i] = append(cpy[i], zeroValue...)
	}
	return cpy
}

// todo: We'll need a more efficient version of this method in production
func (t *MerkleTree) GetProofForLeafAtIndex(forLeaf, untilLeaf uint64) ([]Hash, error) {
	if forLeaf >= uint64(len(t.leaves)) {
		return nil, ErrLeafNotFound
	}
	if untilLeaf >= uint64(len(t.leaves)) {
		return nil, ErrLeafNotFound
	}

	if forLeaf > untilLeaf {
		return nil, errors.New("forLeaf is after untilLeaf")
	}

	layers := make([][]Hash, t.levels)
	currentLayer := t.leaves[:untilLeaf+1]
	for i := 0; i < int(t.levels); i++ {
		if len(currentLayer)%2 != 0 {
			currentLayer = safeAppendToLayer(currentLayer, t.zeroValues[i])
		}

		layers[i] = currentLayer
		currentLayer = hashPairs(currentLayer)
	}

	proof := make([]Hash, t.levels)
	currentIndex := forLeaf

	for i := 0; i < int(t.levels); i++ {
		var sibling Hash
		if currentIndex%2 == 0 {
			sibling = layers[i][currentIndex+1]
		} else {
			sibling = layers[i][currentIndex-1]
		}
		proof[i] = sibling

		currentIndex = currentIndex / 2
	}

	return proof, nil
}

func (t *MerkleTree) String() string {
	var res string
	for i := 0; i < int(t.levels); i++ {
		res += fmt.Sprintf("Level %d: %s\n", i, hex.EncodeToString(t.filledSubtrees[i]))
	}
	res += fmt.Sprintf("Root: %s\n", hex.EncodeToString(t.root))
	return res
}

func calculateZeroValues(levels uint8, seeds []Seed) []Hash {
	zeros := make([]Hash, 0)

	var combinedSeed Seed
	for _, seed := range seeds {
		combinedSeed = append(combinedSeed, seed...)
	}

	current := hash(combinedSeed)
	for i := 0; i < int(levels); i++ {
		current = hashLeftRight(current, current)
		zeros = append(zeros, current)
	}
	return zeros
}

func hashLeftRight(left, right Hash) Hash {
	var combined Hash
	if bytes.Compare(left, right) < 0 {
		combined = safeCombineHashes(left, right)
	} else {
		combined = safeCombineHashes(right, left)
	}
	return hash(combined)
}

func hash(value []byte) Hash {
	hasher := sha256.New()
	hasher.Write(value)
	return hasher.Sum(nil)
}

func hashPairs(layer []Hash) []Hash {
	var res []Hash
	for i := 0; i < len(layer); i += 2 {
		left := layer[i]
		right := layer[i+1]
		hashed := hashLeftRight(left, right)
		res = append(res, hashed)
	}
	return res
}

func safeCombineHashes(hashes ...Hash) []byte {
	var res []byte
	for _, hash := range hashes {
		res = append(res, hash...)
	}
	return res
}

func safeAppendToLayer(slice []Hash, hashes ...Hash) []Hash {
	var res []Hash
	res = append(res, slice...)
	res = append(res, hashes...)
	return res
}

func Verify(proof []Hash, root Hash, leaf Leaf) bool {
	computedHash := hash(leaf)
	for _, proofElement := range proof {
		computedHash = hashLeftRight(computedHash, proofElement)
	}
	return bytes.Equal(computedHash, root)
}

func (h Hash) String() string {
	return hex.EncodeToString(h)
}
