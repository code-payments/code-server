package cvm

import "fmt"

const (
	MinMerkleTreeSize = 1
)

type MerkleTree struct {
	Root      Hash
	Levels    uint8
	NextIndex uint64

	FilledSubtrees HashArray
	ZeroValues     HashArray
}

func (obj *MerkleTree) Unmarshal(data []byte) error {
	if len(data) < MinMerkleTreeSize {
		return ErrInvalidAccountData
	}

	var offset int

	getHash(data, &obj.Root, &offset)
	getUint8(data, &obj.Levels, &offset)

	if len(data) < GetMerkleTreeSize(int(obj.Levels)) {
		return ErrInvalidAccountData
	}

	getUint64(data, &obj.NextIndex, &offset)
	getHashArray(data, &obj.FilledSubtrees, &offset)
	getHashArray(data, &obj.ZeroValues, &offset)

	return nil
}

func (obj *MerkleTree) String() string {
	return fmt.Sprintf(
		"MerkleTree{root=%s,levels=%d,next_index=%d,filled_subtrees=%s,zero_values=%s}",
		obj.Root.String(),
		obj.Levels,
		obj.NextIndex,
		obj.FilledSubtrees.String(),
		obj.ZeroValues.String(),
	)
}

func getMerkleTree(src []byte, dst *MerkleTree, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += GetMerkleTreeSize(int(dst.Levels))
}

func GetMerkleTreeSize(levels int) int {
	return (HashSize + // root
		1 + // levels
		8 + // next_index
		4 + levels*HashSize + // filled_subtrees
		4 + levels*HashSize) // zero_values
}
