package merkletree

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerkleTree_HappyPath_MoreExtensiveWithFewerLevels(t *testing.T) {
	levels := uint8(4)
	seeds := []Seed{Seed("test_seed")}

	tree, err := New(levels, seeds)
	require.NoError(t, err)

	var leaves [][]byte
	for i := 0; i < int(math.Pow(2, float64(levels))); i++ {
		leaf := []byte(fmt.Sprintf("leaf%d", i))
		leaves = append(leaves, leaf)
	}

	roots := make([][]byte, 0)
	for i, leaf := range leaves {
		_, err = tree.GetIndexForLeaf(leaf)
		assert.Equal(t, ErrLeafNotFound, err)

		require.NoError(t, tree.AddLeaf(leaf))
		assert.EqualValues(t, i+1, tree.GetLeafCount())

		index, err := tree.GetIndexForLeaf(leaf)
		require.NoError(t, err)
		assert.Equal(t, i, index)

		roots = append(roots, tree.GetRoot())

		for untilLeaf := 0; untilLeaf < int(tree.GetLeafCount()); untilLeaf++ {
			for forLeaf := 0; forLeaf < untilLeaf; forLeaf++ {
				// Calculate all possible proofs
				proof, err := tree.GetProofForLeafAtIndex(uint64(forLeaf), uint64(untilLeaf))
				require.NoError(t, err)

				for _, root := range roots {
					for _, leaf := range leaves {
						// Check the proof against all root and leaf combinations
						expected := bytes.Equal(root, roots[untilLeaf]) && bytes.Equal(leaf, leaves[forLeaf])
						assert.Equal(t, expected, Verify(proof, root, leaf))
					}
				}
			}
		}
	}

	assert.Equal(t, ErrMerkleTreeFull, tree.AddLeaf([]byte("leaf")))
}

func TestMerkleTree_HappyPath_LessExtensiveWithMoreLevels(t *testing.T) {
	levels := uint8(8)
	seeds := []Seed{Seed("test_seed")}

	tree, err := New(levels, seeds)
	require.NoError(t, err)

	var leaves [][]byte
	for i := 0; i < int(math.Pow(2, float64(levels))); i++ {
		leaf := []byte(fmt.Sprintf("leaf%d", i))
		leaves = append(leaves, leaf)
	}

	for untilLeaf, leaf := range leaves {
		require.NoError(t, tree.AddLeaf(leaf))

		for forLeaf := 0; forLeaf < untilLeaf; forLeaf++ {
			// Calculate and verify the proof for every leaf
			proof, err := tree.GetProofForLeafAtIndex(uint64(forLeaf), uint64(untilLeaf))
			require.NoError(t, err)
			assert.True(t, Verify(proof, tree.GetRoot(), leaves[forLeaf]))
		}
	}
}
