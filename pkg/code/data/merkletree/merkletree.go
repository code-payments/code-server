package merkletree

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/pkg/errors"
)

var (
	ErrMerkleTreeNotFound = errors.New("merkle tree not found")
	ErrLeafNotFound       = errors.New("merkle tree leaf not found")
	ErrRootNotFound       = errors.New("merkle tree root not found")
	ErrStaleMerkleTree    = errors.New("merkle tree state is stale")
	ErrMerkleTreeFull     = errors.New("merkle tree is full")
	ErrInvalidLevelCount  = errors.New("merkle tree level count is invalid")
	ErrTreeIsntMutable    = errors.New("merkle tree is a read-only instance")
)

type Hash []byte
type Leaf []byte
type Seed []byte

const (
	// todo: 64 levels causes an overflow issue
	MaxLevels = 63
	MinLevels = 1

	hashSize = 32
)

const (
	metricsStructName = "data.merkle_tree"
)

// MerkleTree is a DB-backed merkle tree implementation. It is locally thread safe,
// but any higher level entity using this implementation should use a distributed
// lock.
//
// todo: We'll need a heuristic to allow us to delete old versions of nodes whenever
// proofs are no longer needed.
type MerkleTree struct {
	mu             sync.RWMutex
	db             Store
	readOnly       bool
	mtdt           *Metadata
	filledSubtrees []Hash
	zeroValues     []Hash
}

// InitializeNew initializes a new MerkleTree backed by the provided DB
func InitializeNew(ctx context.Context, db Store, name string, levels uint8, seeds []Seed, readOnly bool) (*MerkleTree, error) {
	if levels < MinLevels {
		return nil, ErrInvalidLevelCount
	}
	if levels > MaxLevels {
		return nil, ErrInvalidLevelCount
	}

	var combinedSeed Seed
	for _, seed := range seeds {
		combinedSeed = append(combinedSeed, seed...)
	}

	mtdt := &Metadata{
		Name:      name,
		Levels:    levels,
		NextIndex: 0,
		Seed:      combinedSeed,
		CreatedAt: time.Now(),
	}

	err := db.Create(ctx, mtdt)
	if err != nil {
		return nil, err
	}

	return &MerkleTree{
		db:             db,
		readOnly:       readOnly,
		mtdt:           mtdt,
		filledSubtrees: calculateZeroValues(levels, combinedSeed),
		zeroValues:     calculateZeroValues(levels, combinedSeed),
	}, nil
}

// LoadExisting loads an existing MerkleTree from the provided DB
func LoadExisting(ctx context.Context, db Store, name string, readOnly bool) (*MerkleTree, error) {
	mtdt, err := db.GetByName(ctx, name)
	if err == ErrMetadataNotFound {
		return nil, ErrMerkleTreeNotFound
	}

	filledSubtrees := calculateZeroValues(mtdt.Levels, mtdt.Seed)
	if mtdt.NextIndex > 0 {
		versionToQuery := mtdt.NextIndex
		nodes, err := db.GetLatestNodesForFilledSubtrees(ctx, mtdt.Id, mtdt.Levels, versionToQuery)
		if err != nil {
			return nil, err
		}

		if len(nodes) != int(mtdt.Levels) {
			return nil, errors.New("got invalid number of filled subtrees")
		}

		for level := uint8(0); level < mtdt.Levels; level++ {
			var found bool
			for _, node := range nodes {
				if node.Level == level {
					filledSubtrees[level] = node.Hash
					found = true
					break
				}
			}

			if !found {
				return nil, errors.New("filled subtree node missing")
			}
		}
	}

	return &MerkleTree{
		db:             db,
		readOnly:       readOnly,
		mtdt:           mtdt,
		filledSubtrees: filledSubtrees,
		zeroValues:     calculateZeroValues(mtdt.Levels, mtdt.Seed),
	}, nil
}

// AddLeaf adds a leaf node to the merkle tree. If ErrStaleMerkleTree is returned,
// then this merkle tree instance is outdated and a new one should be loaded.
func (t *MerkleTree) AddLeaf(ctx context.Context, leaf Leaf) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AddLeaf")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	if t.readOnly {
		return ErrTreeIsntMutable
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.mtdt.NextIndex >= uint64(math.Pow(2, float64(t.mtdt.Levels))) {
		return ErrMerkleTreeFull
	}

	// Maintain a temporary copy of the next state
	nextVersion := t.mtdt.NextIndex + 1
	nextMtdt := t.mtdt.Clone()
	nextMtdt.NextIndex += 1
	nextFilledSubtrees := make([]Hash, t.mtdt.Levels)

	leafNode := &Node{
		TreeId:    t.mtdt.Id,
		Level:     0,
		Index:     t.mtdt.NextIndex,
		Hash:      hash(leaf),
		LeafValue: leaf,
		Version:   nextVersion,
	}
	toRoot := make([]*Node, t.mtdt.Levels)

	// Perform the leaf insertion and keep track of intermediary state changes
	var left, right Hash
	currentLevelIndex := t.mtdt.NextIndex
	currentLevelHash := hash(leaf)
	for level := uint8(0); level < t.mtdt.Levels; level++ {
		nextFilledSubtrees[level] = t.filledSubtrees[level]

		if currentLevelIndex%2 == 0 {
			left = currentLevelHash
			right = t.zeroValues[level]
			nextFilledSubtrees[level] = currentLevelHash
		} else {
			left = t.filledSubtrees[level]
			right = currentLevelHash
		}

		currentLevelHash = hashLeftRight(left, right)
		currentLevelIndex = currentLevelIndex / 2

		nodeAtOneLevelAbove := &Node{
			TreeId:  t.mtdt.Id,
			Level:   level + 1,
			Index:   currentLevelIndex,
			Hash:    currentLevelHash,
			Version: nextVersion,
		}
		toRoot[level] = nodeAtOneLevelAbove
	}

	// Update the DB with the new leaf and nodes along the path from leaf to root
	err := t.db.AddLeaf(ctx, &nextMtdt, leafNode, toRoot)
	if err == ErrInvalidMetadata {
		return ErrStaleMerkleTree
	} else if err != nil {
		return errors.Wrap(err, "error adding leaf")
	}

	// Update local state only when DB has been updated, so we maintain a
	// consistent state
	t.mtdt = &nextMtdt
	t.filledSubtrees = nextFilledSubtrees

	return nil
}

// SimulateAddingLeaves simulates adding leaves to the merkle tree given its
// local state and returns the computed root hash. This can be used to validate
// leaf insertion before committing it to the DB using AddLeaf.
func (t *MerkleTree) SimulateAddingLeaves(ctx context.Context, leaves []Leaf) (Hash, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "SimulateAddingLeaves")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	if len(leaves) == 0 {
		return nil, errors.New("must simulate with at least one leaf")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	fillSubtreesCopy := make([]Hash, len(t.filledSubtrees))
	copy(fillSubtreesCopy, t.filledSubtrees)

	var root Hash
	for i, leaf := range leaves {
		nextIndex := t.mtdt.NextIndex + uint64(i)
		if nextIndex >= uint64(math.Pow(2, float64(t.mtdt.Levels))) {
			return nil, ErrMerkleTreeFull
		}

		var left, right Hash
		currentLevelIndex := nextIndex
		currentLevelHash := hash(leaf)
		for level := uint8(0); level < t.mtdt.Levels; level++ {
			if currentLevelIndex%2 == 0 {
				left = currentLevelHash
				right = t.zeroValues[level]
				fillSubtreesCopy[level] = currentLevelHash
			} else {
				left = fillSubtreesCopy[level]
				right = currentLevelHash
			}

			currentLevelHash = hashLeftRight(left, right)
			currentLevelIndex = currentLevelIndex / 2
		}

		root = currentLevelHash
	}

	return root, nil
}

// GetCurrentRootNode gets the current root node given the tree's local
// state
func (t *MerkleTree) GetCurrentRootNode(ctx context.Context) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetCurrentRootNode")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	// Edge case for an empty tree, where we have no data and the root node is
	// the zero value.
	if t.mtdt.NextIndex == 0 {
		return &Node{
			TreeId:  t.mtdt.Id,
			Level:   t.mtdt.Levels,
			Index:   0,
			Hash:    t.zeroValues[len(t.zeroValues)-1],
			Version: 0,
		}, nil
	}

	versionToQuery := t.mtdt.NextIndex
	rootNode, err := t.db.GetNode(ctx, t.mtdt.Id, t.mtdt.Levels, 0, versionToQuery)
	if err == ErrNodeNotFound {
		return nil, ErrRootNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting root node")
	}

	return rootNode, nil
}

// GetRootNodeForLeaf gets the root node at the time of adding the provided leaf
func (t *MerkleTree) GetRootNodeForLeaf(ctx context.Context, leaf uint64) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetRootNodeForLeaf")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	versionToQuery := leaf + 1
	rootNode, err := t.db.GetNode(ctx, t.mtdt.Id, t.mtdt.Levels, 0, versionToQuery)
	if err == ErrNodeNotFound {
		return nil, ErrRootNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting root node")
	}

	return rootNode, nil
}

// GetLeafCount gets the number of leaves in the merkle tree given the tree's
// local state
func (t *MerkleTree) GetLeafCount(ctx context.Context) (uint64, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetLeafCount")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.mtdt.NextIndex, nil
}

// GetLastAddedLeafNode gets the last added leaf node given the tree's local
// state
func (t *MerkleTree) GetLastAddedLeafNode(ctx context.Context) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetLastAddedLeafNode")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.mtdt.NextIndex == 0 {
		return nil, ErrLeafNotFound
	}

	versionToQuery := t.mtdt.NextIndex
	leafNode, err := t.db.GetNode(ctx, t.mtdt.Id, 0, t.mtdt.NextIndex-1, versionToQuery)
	if err != nil {
		return nil, errors.Wrap(err, "error getting leaf node")
	}

	return leafNode, nil
}

// GetLeafNode gets a leaf node in the tree by its value
func (t *MerkleTree) GetLeafNode(ctx context.Context, leaf Leaf) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetLeafNode")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	leafNode, err := t.db.GetLeafByHash(ctx, t.mtdt.Id, hash(leaf))
	if err == ErrNodeNotFound {
		return nil, ErrLeafNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting leaf node")
	}

	return leafNode, nil
}

func (t *MerkleTree) GetLeafNodeByIndex(ctx context.Context, index uint64) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetLeafNode")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	versionToQuery := index + 1
	leafNode, err := t.db.GetNode(ctx, t.mtdt.Id, 0, index, versionToQuery)
	if err == ErrNodeNotFound {
		return nil, ErrLeafNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting leaf node by index")
	}

	return leafNode, nil
}

// GetLeafNodeForRoot gets the leaf node added into the tree when the root equated
// to the provided hash.
func (t *MerkleTree) GetLeafNodeForRoot(ctx context.Context, root Hash) (*Node, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetLeafNodeForRoot")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	rootNode, err := t.db.GetNodeByHash(ctx, t.mtdt.Id, root)
	if err == ErrNodeNotFound {
		return nil, ErrRootNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting root node")
	}

	if rootNode.Level != t.mtdt.Levels {
		return nil, ErrRootNotFound
	}

	leafNode, err := t.db.GetNode(ctx, t.mtdt.Id, 0, rootNode.Version-1, rootNode.Version)
	if err == ErrNodeNotFound {
		return nil, ErrLeafNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting leaf node")
	}

	return leafNode, nil
}

// GetProofForLeafAtIndex gets a merkle proof for a leaf node
func (t *MerkleTree) GetProofForLeafAtIndex(ctx context.Context, forLeaf, untilLeaf uint64) ([]Hash, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetProofForLeafAtIndex")
	tracer.AddAttributes(map[string]interface{}{
		"name":      t.mtdt.Name,
		"forLeaf":   forLeaf,
		"untilLeaf": untilLeaf,
	})
	defer tracer.End()

	t.mu.RLock()
	defer t.mu.RUnlock()

	if forLeaf > untilLeaf {
		return nil, errors.New("forLeaf is after untilLeaf")
	}

	if untilLeaf >= t.mtdt.NextIndex {
		return nil, ErrLeafNotFound
	}

	versionToQuery := untilLeaf + 1
	nodes, err := t.db.GetLatestNodesForProof(ctx, t.mtdt.Id, t.mtdt.Levels, forLeaf, versionToQuery)
	if err != nil {
		return nil, err
	}

	proof := make([]Hash, t.mtdt.Levels)
	for level := uint8(0); level < t.mtdt.Levels; level++ {
		var siblingHash Hash
		for _, node := range nodes {
			if node.Level == level {
				siblingHash = node.Hash
				break
			}
		}

		if siblingHash == nil {
			siblingHash = t.zeroValues[level]
		}

		proof[level] = siblingHash
	}

	return proof, nil
}

// Refresh refreshes a MerkleTree to an updated state. This is particularly
// useful in certain scenarios:
//  1. The tree is in read-only mode, and won't observe state changes locally.
//  2. AddLeaf failed with ErrStaleMerkleTree, so it must be outdated.
func (t *MerkleTree) Refresh(ctx context.Context) error {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "Refresh")
	tracer.AddAttribute("name", t.mtdt.Name)
	defer tracer.End()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Reuse the LoadExisting function
	updatedMerkleTree, err := LoadExisting(ctx, t.db, t.mtdt.Name, t.readOnly)
	if err != nil {
		return err
	}

	// Steal its updated state
	t.mtdt = updatedMerkleTree.mtdt
	t.filledSubtrees = updatedMerkleTree.filledSubtrees

	return nil
}

func calculateZeroValues(levels uint8, seed Seed) []Hash {
	zeros := make([]Hash, 0)
	current := hash(seed)
	for i := 0; i < int(levels); i++ {
		current = hashLeftRight(current, current)
		zeros = append(zeros, current)
	}
	return zeros
}

func hash(value []byte) Hash {
	hasher := sha256.New()
	hasher.Write(value)
	return hasher.Sum(nil)
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

func safeCombineHashes(hashes ...Hash) []byte {
	var res []byte
	for _, hash := range hashes {
		res = append(res, hash...)
	}
	return res
}

// Verify verifies that a merkle proof is correct
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
