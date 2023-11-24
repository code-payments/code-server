package memory

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

type store struct {
	mu              sync.Mutex
	metadataRecords []*merkletree.Metadata
	nodeRecords     []*merkletree.Node
	last            uint64
}

func New() merkletree.Store {
	return &store{
		metadataRecords: make([]*merkletree.Metadata, 0),
		nodeRecords:     make([]*merkletree.Node, 0),
		last:            0,
	}
}

// Create implements merkletree.Store.Create
func (s *store) Create(_ context.Context, mtdt *merkletree.Metadata) error {
	if err := merkletree.ValidateCreate(mtdt); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	if item := s.findMetadata(mtdt); item != nil {
		return merkletree.ErrMetadataExists
	}

	mtdt.Id = s.last
	if mtdt.CreatedAt.IsZero() {
		mtdt.CreatedAt = time.Now()
	}

	cloned := mtdt.Clone()
	s.metadataRecords = append(s.metadataRecords, &cloned)

	return nil
}

// GetByName implements merkletree.Store.GetByName
func (s *store) GetByName(_ context.Context, name string) (*merkletree.Metadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findMetadataByName(name)
	if item == nil {
		return nil, merkletree.ErrMetadataNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// AddLeaf implements merkletree.Store.AddLeaf
func (s *store) AddLeaf(_ context.Context, mtdt *merkletree.Metadata, leaf *merkletree.Node, toRoot []*merkletree.Node) error {
	if err := merkletree.ValidateAddLeaf(mtdt, leaf, toRoot); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	metadataItem := s.findMetadata(mtdt)
	if metadataItem == nil {
		return merkletree.ErrMetadataNotFound
	} else if metadataItem.NextIndex+1 != mtdt.NextIndex {
		return merkletree.ErrInvalidMetadata
	}

	var allNodes []*merkletree.Node
	allNodes = append(allNodes, leaf)
	allNodes = append(allNodes, toRoot...)

	for _, node := range allNodes {
		if nodeItem := s.findNode(node); nodeItem != nil {
			return merkletree.ErrNodeExists
		}
	}

	metadataItem.NextIndex = mtdt.NextIndex
	metadataItem.CopyTo(mtdt)

	for _, node := range allNodes {
		s.last++

		node.Id = s.last
		if node.CreatedAt.IsZero() {
			node.CreatedAt = time.Now()
		}
		cloned := node.Clone()

		s.nodeRecords = append(s.nodeRecords, &cloned)
	}

	return nil
}

// GetNode implements merkletree.Store.GetNode
func (s *store) GetNode(_ context.Context, tree uint64, level uint8, index uint64, version uint64) (*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findNodeByVersion(tree, level, index, version)
	if item == nil {
		return nil, merkletree.ErrNodeNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetLatestNode implements merkletree.Store.GetLatestNode
func (s *store) GetLatestNode(_ context.Context, tree uint64, level uint8, index uint64, untilVersion uint64) (*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findNodeUntilVersion(tree, level, index, untilVersion)
	if len(items) == 0 {
		return nil, merkletree.ErrNodeNotFound
	}

	cloned := items[len(items)-1].Clone()
	return &cloned, nil
}

// GetLatestNodesForProof implements merkletree.Store.GetLatestNodesForProof
func (s *store) GetLatestNodesForProof(ctx context.Context, tree uint64, levels uint8, forLeaf, untilVersion uint64) ([]*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res []*merkletree.Node
	currentLevelIndex := forLeaf
	for level := uint8(0); level < levels; level++ {
		var siblingIndex uint64
		if currentLevelIndex%2 == 0 {
			siblingIndex = currentLevelIndex + 1
		} else {
			siblingIndex = currentLevelIndex - 1
		}

		items := s.findNodeUntilVersion(tree, level, siblingIndex, untilVersion)
		if len(items) != 0 {
			res = append(res, items[len(items)-1])
		}

		currentLevelIndex = currentLevelIndex / 2
	}

	return res, nil
}

// GetLatestNodesForFilledSubtrees implements merkletree.Store.GetLatestNodesForFilledSubtrees
func (s *store) GetLatestNodesForFilledSubtrees(ctx context.Context, tree uint64, levels uint8, untilVersion uint64) ([]*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res []*merkletree.Node
	currentLevelIndex := untilVersion - 1
	for level := uint8(0); level < levels; level++ {
		nodeIndex := currentLevelIndex
		if nodeIndex%2 != 0 {
			nodeIndex = nodeIndex - 1
		}

		items := s.findNodeUntilVersion(tree, level, nodeIndex, untilVersion)
		if len(items) != 0 {
			res = append(res, items[len(items)-1])
		}

		currentLevelIndex = currentLevelIndex / 2
	}
	return res, nil
}

// GetLeafByHash implements merkletree.Store.GetLeafByHash
func (s *store) GetLeafByHash(_ context.Context, tree uint64, hash merkletree.Hash) (*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findNodesByTree(tree)
	items = s.filterNodesByLevel(items, 0)
	items = s.filterNodesByHash(items, hash)

	if len(items) == 0 {
		return nil, merkletree.ErrNodeNotFound
	}

	cloned := items[len(items)-1].Clone()
	return &cloned, nil
}

// GetNodeByHash implements merkletree.Store.GetNodeByHash
func (s *store) GetNodeByHash(_ context.Context, tree uint64, hash merkletree.Hash) (*merkletree.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findNodesByTree(tree)
	items = s.filterNodesByHash(items, hash)

	if len(items) == 0 {
		return nil, merkletree.ErrNodeNotFound
	}

	cloned := items[len(items)-1].Clone()
	return &cloned, nil
}

func (s *store) findMetadata(data *merkletree.Metadata) *merkletree.Metadata {
	for _, item := range s.metadataRecords {
		if item.Id == data.Id {
			return item
		}

		if item.Name == data.Name {
			return item
		}
	}
	return nil
}

func (s *store) findMetadataByName(name string) *merkletree.Metadata {
	for _, item := range s.metadataRecords {
		if item.Name == name {
			return item
		}
	}
	return nil
}

func (s *store) findNode(data *merkletree.Node) *merkletree.Node {
	for _, item := range s.nodeRecords {
		if item.Id == data.Id {
			return item
		}

		if item.TreeId != data.TreeId {
			continue
		}

		if item.Level != data.Level {
			continue
		}

		if item.Index != data.Index {
			continue
		}

		if item.Version != data.Version {
			continue
		}

		return item
	}
	return nil
}

func (s *store) findNodeByVersion(tree uint64, level uint8, index uint64, version uint64) *merkletree.Node {
	for _, item := range s.nodeRecords {
		if item.TreeId != tree {
			continue
		}

		if item.Level != level {
			continue
		}

		if item.Index != index {
			continue
		}

		if item.Version != version {
			continue
		}

		return item
	}
	return nil
}

func (s *store) findNodeUntilVersion(tree uint64, level uint8, index uint64, version uint64) []*merkletree.Node {
	var res []*merkletree.Node

	for _, item := range s.nodeRecords {
		if item.TreeId != tree {
			continue
		}

		if item.Level != level {
			continue
		}

		if item.Index != index {
			continue
		}

		if item.Version > version {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) findNodesByTree(tree uint64) []*merkletree.Node {
	var res []*merkletree.Node
	for _, item := range s.nodeRecords {
		if item.TreeId == tree {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterNodesByLevel(items []*merkletree.Node, level uint8) []*merkletree.Node {
	var res []*merkletree.Node
	for _, item := range items {
		if item.Level == level {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterNodesByHash(items []*merkletree.Node, hash merkletree.Hash) []*merkletree.Node {
	var res []*merkletree.Node
	for _, item := range items {
		if bytes.Equal(item.Hash, hash) {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	s.metadataRecords = make([]*merkletree.Metadata, 0)
	s.nodeRecords = make([]*merkletree.Node, 0)
	s.last = 0
	s.mu.Unlock()
}
