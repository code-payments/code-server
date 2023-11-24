package splitter_token

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	MerkleTreePrefix = []byte("merkletree")
)

// On-chain MerkleTree, which does not include leaves.
type MerkleTree struct {
	Levels         uint8
	NextIndex      uint64
	Root           Hash
	FilledSubtrees []Hash
	ZeroValues     []Hash
}

// The size of the {@link MerkleTree} in bytes for the static portion
const MerkleTreeSize = (1 + // levels
	4 + // filled_subtrees length
	4 + // zero_values length
	8 + // next_index
	HashSize) // root

// The size of the {@link MerkleTree} in bytes including the dynamic portion
func getMerkleTreeSize(levels uint8) int {
	return (MerkleTreeSize + // MerkleTree
		int(levels)*HashSize + // filled_subtrees
		int(levels)*HashSize) // zero_values
}

// Holds the data for the {@link MerkleTree} and provides de/serialization
// functionality for that data
func NewMerkleTree(
	levels uint8,
	nextIndex uint64,
	root Hash,
	filledSubtrees []Hash,
	zeroValues []Hash,
) *MerkleTree {
	return &MerkleTree{
		Levels:         levels,
		NextIndex:      nextIndex,
		Root:           root,
		FilledSubtrees: filledSubtrees,
		ZeroValues:     zeroValues,
	}
}

// Clones a {@link MerkleTree} instance.
func (obj *MerkleTree) Clone() *MerkleTree {
	filledSubtrees := make([]Hash, len(obj.FilledSubtrees))
	zeroValues := make([]Hash, len(obj.ZeroValues))

	copy(filledSubtrees, obj.FilledSubtrees)
	copy(zeroValues, obj.ZeroValues)

	return NewMerkleTree(
		obj.Levels,
		obj.NextIndex,
		obj.Root,
		filledSubtrees,
		zeroValues,
	)
}

func (obj *MerkleTree) ToString() string {
	hashToString := func(list []Hash) string {
		var s string
		for _, h := range list {
			s += fmt.Sprintf("'%x', ", h)
		}
		return s
	}
	return "MerkleTree {" +
		"  levels: '" + strconv.Itoa(int(obj.Levels)) + "'" +
		", next_index: '" + strconv.FormatUint(obj.NextIndex, 10) + "'" +
		", root: '" + obj.Root.ToString() + "'" +
		", filled_subtrees: [\n" + hashToString(obj.FilledSubtrees) + "]" +
		", zero_values: [\n" + hashToString(obj.ZeroValues) + "]" +
		"}"
}

// Serializes the {@link MerkleTree} into a Buffer.
// @returns the created []byte buffer
func (obj *MerkleTree) Marshal() []byte {
	data := make([]byte, getMerkleTreeSize(obj.Levels))

	var offset int = 0

	putUint8(data, obj.Levels, &offset)
	putUint64(data, obj.NextIndex, &offset)
	putHash(data, obj.Root[:], &offset)

	putUint32(data, uint32(obj.Levels), &offset)
	for _, subtree := range obj.FilledSubtrees {
		putHash(data, subtree[:], &offset)
	}

	putUint32(data, uint32(obj.Levels), &offset)
	for _, zeroValue := range obj.ZeroValues {
		putHash(data, zeroValue[:], &offset)
	}

	return nil
}

// Deserializes the {@link MerkleTree} from the provided data Buffer.
// @returns an error if the deserialize operation was unsuccessful.
func (obj *MerkleTree) Unmarshal(data []byte) error {
	var offset int = 0

	getUint8(data, &obj.Levels, &offset)

	if len(data) < getMerkleTreeSize(obj.Levels) {
		return ErrInvalidInstructionData
	}

	getUint64(data, &obj.NextIndex, &offset)
	getHash(data, &obj.Root, &offset)

	var length uint32
	getUint32(data, &length, &offset)
	if uint32(obj.Levels) != length {
		return errors.New("invalid filled_subtrees length")
	}

	obj.FilledSubtrees = make([]Hash, obj.Levels)
	for i := uint8(0); i < obj.Levels; i++ {
		getHash(data, &obj.FilledSubtrees[i], &offset)
	}

	getUint32(data, &length, &offset)
	if uint32(obj.Levels) != length {
		return errors.New("invalid zero_values length")
	}

	obj.ZeroValues = make([]Hash, obj.Levels)
	for i := uint8(0); i < obj.Levels; i++ {
		getHash(data, &obj.ZeroValues[i], &offset)
	}

	return nil
}
