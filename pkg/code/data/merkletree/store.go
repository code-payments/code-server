package merkletree

import (
	"context"

	"github.com/pkg/errors"
)

var (
	ErrMetadataExists   = errors.New("merkle tree metadata already exists")
	ErrMetadataNotFound = errors.New("merkle tree metadata not found")
	ErrInvalidMetadata  = errors.New("merkle tree metadata is old or invalid")
	ErrNodeExists       = errors.New("merkle tree node already exists")
	ErrNodeNotFound     = errors.New("merkle tree node not found")
)

// Store is a DB layer to support persistent merkle trees. It should be used in
// conjuction with the MerkleTree implementation (ie. don't use this interface
// directly).
type Store interface {
	// Create creates a new merkle tree metadata
	Create(ctx context.Context, mtdt *Metadata) error

	// GetByName gets merkle tree metadata by name
	GetByName(ctx context.Context, name string) (*Metadata, error)

	// AddLeaf updates a merkle tree by inserting the provided leaf and
	// updated nodes going along the path from leaf to root.
	//
	// todo: Better name for "toRoot"
	AddLeaf(ctx context.Context, mtdt *Metadata, leaf *Node, toRoot []*Node) error

	// GetNode gets an exactly versioned node for a tree at a particular level
	// and index
	GetNode(ctx context.Context, tree uint64, level uint8, index uint64, version uint64) (*Node, error)

	// GetLatestNode gets the latest versioned node, bounded inclusively by
	// untilVersion, for a tree at a particular level and index
	GetLatestNode(ctx context.Context, tree uint64, level uint8, index uint64, untilVersion uint64) (*Node, error)

	// GetLatestNodesForProof gets the latest non-zero nodes that make up the
	// proof for forLeaf at the untilVersion version of the merklee tree
	GetLatestNodesForProof(ctx context.Context, tree uint64, levels uint8, forLeaf, untilVersion uint64) ([]*Node, error)

	// GetLatestNodesForFilledSubtrees gets the latest nodes that make up a
	// merkle tree's filled subtrees array at the untilVersion version of the
	// merklee tree
	GetLatestNodesForFilledSubtrees(ctx context.Context, tree uint64, levels uint8, untilVersion uint64) ([]*Node, error)

	// GetLeafByHash gets a leaf node by its hash value
	GetLeafByHash(ctx context.Context, tree uint64, hash Hash) (*Node, error)

	// GetNodeByHash gets a node by its hash value
	GetNodeByHash(ctx context.Context, tree uint64, hash Hash) (*Node, error)
}

// ValidateCreate is used by Store implementations to validate Create calls
func ValidateCreate(mtdt *Metadata) error {
	if err := mtdt.Validate(); err != nil {
		return errors.Wrap(err, "tree metadata failed general validation")
	}

	if mtdt.NextIndex != 0 {
		return errors.New("next index should be 0")
	}

	return nil
}

// ValidateAddLeaf is used by Store implementations to validate AddLeaf calls
func ValidateAddLeaf(mtdt *Metadata, leaf *Node, toRoot []*Node) error {
	expectedVersion := mtdt.NextIndex

	// metadata validation

	if err := mtdt.Validate(); err != nil {
		return errors.Wrap(err, "tree metadata failed general validation")
	}

	if mtdt.NextIndex == 0 {
		return errors.New("next index should be greater than zero")
	}

	// leaf validation

	if err := leaf.Validate(); err != nil {
		return errors.Wrap(err, "leaf failed general validation")
	}

	if leaf.TreeId != mtdt.Id {
		return errors.New("invalid tree id for leaf")
	}

	if leaf.Level != 0 {
		return errors.New("invalid leaf level")
	}

	if leaf.Index != mtdt.NextIndex-1 {
		return errors.New("invalid leaf index")
	}

	if leaf.Version != expectedVersion {
		return errors.New("invalid leaf version")
	}

	// toRoot validation

	if len(toRoot) != int(mtdt.Levels) {
		return errors.New("invalid toRoot size")
	}

	currentNodeIndex := leaf.Index / 2
	for i, node := range toRoot {
		if err := node.Validate(); err != nil {
			return errors.Wrap(err, "toRoot node failed general validation")
		}

		if node.TreeId != mtdt.Id {
			return errors.New("invalid tree id for toRoot node")
		}

		if node.Level != uint8(i+1) {
			return errors.New("invalid toRoot node level")
		}

		if node.Index != currentNodeIndex {
			return errors.New("invalid toRoot node index")
		}

		if node.Version != expectedVersion {
			return errors.New("invalid toRoot node version")
		}

		currentNodeIndex = currentNodeIndex / 2
	}

	return nil
}
