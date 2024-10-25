package cvm

import "fmt"

const (
	minMerkleTreeSize = (HashSize + // root
		8) // next_index
)

var (
	MerkleTreePrefix = []byte("merkletree")
)

type MerkleTree struct {
	Root           Hash
	FilledSubtrees HashArray
	ZeroValues     HashArray
	NextIndex      uint64
}

func (obj *MerkleTree) Unmarshal(data []byte, levels int) error {
	if len(data) < GetMerkleTreeSize(levels) {
		return ErrInvalidAccountData
	}

	var offset int

	getHash(data, &obj.Root, &offset)
	getStaticHashArray(data, &obj.FilledSubtrees, levels, &offset)
	getStaticHashArray(data, &obj.ZeroValues, levels, &offset)
	getUint64(data, &obj.NextIndex, &offset)

	return nil
}

func (obj *MerkleTree) String() string {
	return fmt.Sprintf(
		"MerkleTree{root=%s,filled_subtrees=%s,zero_values=%s,next_index=%d}",
		obj.Root.String(),
		obj.FilledSubtrees.String(),
		obj.ZeroValues.String(),
		obj.NextIndex,
	)
}

func getMerkleTree(src []byte, dst *MerkleTree, levels int, offset *int) {
	dst.Unmarshal(src[*offset:], levels)
	*offset += GetMerkleTreeSize(int(levels))
}

func GetMerkleTreeSize(levels int) int {
	return (minMerkleTreeSize +
		levels*HashSize + // filled_subtrees
		levels*HashSize) // zero_values
}
