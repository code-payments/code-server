package merkletree

import (
	"errors"
	"time"
)

type Metadata struct {
	Id        uint64
	Name      string
	Levels    uint8
	NextIndex uint64
	Seed      Seed
	CreatedAt time.Time
}

// Level 0 = leaf
// Level (0, Record.Levels) = subtree
// Level Record.Levels = root
type Node struct {
	Id        uint64
	TreeId    uint64
	Level     uint8
	Index     uint64
	Hash      Hash
	LeafValue []byte
	Version   uint64
	CreatedAt time.Time
}

func (m *Metadata) Validate() error {
	if m.Levels > MaxLevels {
		return errors.New("too many levels")
	}

	if m.Levels < MinLevels {
		return errors.New("too few levels")
	}

	if len(m.Seed) == 0 {
		return errors.New("seed is required")
	}

	return nil
}

func (m *Metadata) Clone() Metadata {
	seedCopy := make([]byte, len(m.Seed))
	copy(seedCopy, m.Seed)

	return Metadata{
		Id:        m.Id,
		Name:      m.Name,
		Levels:    m.Levels,
		NextIndex: m.NextIndex,
		Seed:      seedCopy,
		CreatedAt: m.CreatedAt,
	}
}

func (m *Metadata) CopyTo(dst *Metadata) {
	seedCopy := make([]byte, len(m.Seed))
	copy(seedCopy, m.Seed)

	dst.Id = m.Id
	dst.Name = m.Name
	dst.Levels = m.Levels
	dst.NextIndex = m.NextIndex
	dst.Seed = seedCopy
	dst.CreatedAt = m.CreatedAt
}

func (n *Node) Validate() error {
	if n.TreeId == 0 {
		return errors.New("tree id is required")
	}

	if n.Level > MaxLevels+1 {
		return errors.New("too many levels")
	}

	if len(n.Hash) == 0 {
		return errors.New("hash is required")
	}

	if len(n.Hash) != hashSize {
		return errors.New("invalid hash size")
	}

	if n.Level == 0 && len(n.LeafValue) == 0 {
		return errors.New("leaf value must be present for leaf node")
	} else if n.Level != 0 && len(n.LeafValue) != 0 {
		return errors.New("leaf value cannot be set for node")
	}

	return nil
}

func (n *Node) Clone() Node {
	leafValueCopy := make([]byte, len(n.LeafValue))
	copy(leafValueCopy, n.LeafValue)

	return Node{
		Id:        n.Id,
		TreeId:    n.TreeId,
		Level:     n.Level,
		Index:     n.Index,
		Hash:      n.Hash,
		LeafValue: leafValueCopy,
		Version:   n.Version,
		CreatedAt: n.CreatedAt,
	}
}

func (n *Node) CopyTo(dst *Node) {
	leafValueCopy := make([]byte, len(n.LeafValue))
	copy(leafValueCopy, n.LeafValue)

	dst.Id = n.Id
	dst.TreeId = n.TreeId
	dst.Level = n.Level
	dst.Index = n.Index
	dst.Hash = n.Hash
	dst.LeafValue = leafValueCopy
	dst.Version = n.Version
	dst.CreatedAt = n.CreatedAt
}
