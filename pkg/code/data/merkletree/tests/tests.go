package tests

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	reference "github.com/code-payments/code-server/pkg/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

func RunTests(t *testing.T, s merkletree.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s merkletree.Store){
		testHappyPath,
	} {
		tf(t, s)
		teardown()
	}
}

func testHappyPath(t *testing.T, s merkletree.Store) {
	testCases := map[string]struct {
		alwaysLoadFromDB bool
	}{
		"testHappyPath": {
			alwaysLoadFromDB: false,
		},
		"testHappyPathWithDBLoadingBeforeOperation": {
			alwaysLoadFromDB: true,
		},
	}

	for testName, parameters := range testCases {
		t.Run(testName, func(t *testing.T) {
			ctx := context.Background()

			// Part 1: Initialize a new merkle tree

			expectedMetadata := &merkletree.Metadata{
				Name:      testName,
				Levels:    6,
				NextIndex: 0,
				Seed:      merkletree.Seed("seed"),
			}

			_, err := s.GetByName(ctx, expectedMetadata.Name)
			assert.Equal(t, merkletree.ErrMetadataNotFound, err)

			_, err = merkletree.LoadExisting(ctx, s, expectedMetadata.Name, false)
			assert.Equal(t, merkletree.ErrMerkleTreeNotFound, err)

			merkleTree, err := merkletree.InitializeNew(ctx, s, expectedMetadata.Name, expectedMetadata.Levels, []merkletree.Seed{expectedMetadata.Seed}, false)
			require.NoError(t, err)

			_, err = merkletree.InitializeNew(ctx, s, expectedMetadata.Name, expectedMetadata.Levels, []merkletree.Seed{expectedMetadata.Seed}, false)
			assert.Equal(t, merkletree.ErrMetadataExists, err)

			merkleTreeAtInitialState, err := merkletree.LoadExisting(ctx, s, expectedMetadata.Name, true)
			require.NoError(t, err)

			actualMetadata, err := s.GetByName(ctx, expectedMetadata.Name)
			require.NoError(t, err)
			assertEquivalentMetadata(t, expectedMetadata, actualMetadata)

			referenceTree, err := reference.New(expectedMetadata.Levels, []reference.Seed{reference.Seed(expectedMetadata.Seed)})
			require.NoError(t, err)

			// Part 2: Adding leaves into the merkle tree until it's full

			rootNode, err := merkleTree.GetCurrentRootNode(ctx)
			require.NoError(t, err)
			assert.EqualValues(t, referenceTree.GetRoot(), rootNode.Hash)
			assert.Equal(t, expectedMetadata.Levels, rootNode.Level)
			assert.EqualValues(t, 0, rootNode.Index)

			_, err = merkleTree.GetLastAddedLeafNode(ctx)
			assert.Equal(t, merkletree.ErrLeafNotFound, err)

			leafCount, err := merkleTree.GetLeafCount(ctx)
			require.NoError(t, err)
			assert.EqualValues(t, 0, leafCount)

			var allLeaves []merkletree.Leaf
			totalLeafCount := uint64(math.Pow(2, float64(expectedMetadata.Levels)))
			for i := uint64(0); i < totalLeafCount; i++ {
				if parameters.alwaysLoadFromDB {
					merkleTree, err = merkletree.LoadExisting(ctx, s, expectedMetadata.Name, false)
					require.NoError(t, err)
				}

				staleMerkleTree, err := merkletree.LoadExisting(ctx, s, expectedMetadata.Name, false)
				require.NoError(t, err)

				leafValue := fmt.Sprintf("leaf%d", i)
				allLeaves = append(allLeaves, merkletree.Leaf(leafValue))

				_, err = merkleTree.GetLeafNode(ctx, merkletree.Leaf(leafValue))
				assert.Equal(t, merkletree.ErrLeafNotFound, err)

				_, err = merkleTree.GetLeafNodeByIndex(ctx, i)
				assert.Equal(t, merkletree.ErrLeafNotFound, err)

				_, err = merkleTree.GetRootNodeForLeaf(ctx, i)
				assert.Equal(t, merkletree.ErrRootNotFound, err)

				require.NoError(t, referenceTree.AddLeaf(reference.Leaf(leafValue)))

				simulatedRoot, err := staleMerkleTree.SimulateAddingLeaves(ctx, []merkletree.Leaf{merkletree.Leaf(leafValue)})
				require.NoError(t, err)
				assert.EqualValues(t, referenceTree.GetRoot(), simulatedRoot)

				_, err = merkleTree.GetLeafNodeForRoot(ctx, simulatedRoot)
				assert.Equal(t, merkletree.ErrRootNotFound, err)

				require.NoError(t, merkleTree.AddLeaf(ctx, merkletree.Leaf(leafValue)))

				err = staleMerkleTree.AddLeaf(ctx, merkletree.Leaf(leafValue))
				assert.Equal(t, merkletree.ErrStaleMerkleTree, err)

				leafNode, err := merkleTree.GetLeafNode(ctx, merkletree.Leaf(leafValue))
				require.NoError(t, err)
				assert.Equal(t, leafValue, string(leafNode.LeafValue))
				assert.EqualValues(t, 0, leafNode.Level)
				assert.Equal(t, i, leafNode.Index)

				leafNode, err = merkleTree.GetLeafNodeByIndex(ctx, i)
				require.NoError(t, err)
				assert.Equal(t, leafValue, string(leafNode.LeafValue))
				assert.EqualValues(t, 0, leafNode.Level)
				assert.Equal(t, i, leafNode.Index)

				leafNode, err = merkleTree.GetLastAddedLeafNode(ctx)
				require.NoError(t, err)
				assert.Equal(t, leafValue, string(leafNode.LeafValue))
				assert.EqualValues(t, 0, leafNode.Level)
				assert.Equal(t, i, leafNode.Index)

				leafNode, err = merkleTree.GetLeafNodeForRoot(ctx, simulatedRoot)
				require.NoError(t, err)
				assert.Equal(t, leafValue, string(leafNode.LeafValue))
				assert.EqualValues(t, 0, leafNode.Level)
				assert.Equal(t, i, leafNode.Index)

				rootNode, err := merkleTree.GetCurrentRootNode(ctx)
				require.NoError(t, err)
				assert.EqualValues(t, referenceTree.GetRoot(), rootNode.Hash)
				assert.Equal(t, expectedMetadata.Levels, rootNode.Level)
				assert.EqualValues(t, 0, rootNode.Index)

				rootNode, err = merkleTree.GetRootNodeForLeaf(ctx, leafNode.Index)
				require.NoError(t, err)
				assert.EqualValues(t, referenceTree.GetRoot(), rootNode.Hash)
				assert.Equal(t, expectedMetadata.Levels, rootNode.Level)
				assert.EqualValues(t, 0, rootNode.Index)

				leafCount, err := merkleTree.GetLeafCount(ctx)
				require.NoError(t, err)
				assert.EqualValues(t, i+1, leafCount)
			}

			simulatedRoot, err := merkleTreeAtInitialState.SimulateAddingLeaves(ctx, allLeaves)
			require.NoError(t, err)
			assert.EqualValues(t, referenceTree.GetRoot(), simulatedRoot)

			err = merkleTree.AddLeaf(ctx, merkletree.Leaf("too many"))
			assert.Equal(t, merkletree.ErrMerkleTreeFull, err)

			_, err = merkleTree.SimulateAddingLeaves(ctx, []merkletree.Leaf{merkletree.Leaf("too many")})
			assert.Equal(t, merkletree.ErrMerkleTreeFull, err)

			// Part 3: Obtain and validate a proof for all leaf combinations

			for forLeaf := uint64(0); forLeaf < totalLeafCount; forLeaf++ {
				for untilLeaf := forLeaf; untilLeaf < totalLeafCount; untilLeaf++ {
					if parameters.alwaysLoadFromDB {
						merkleTree, err = merkletree.LoadExisting(ctx, s, expectedMetadata.Name, true)
						require.NoError(t, err)
					}

					expectedProof, err := referenceTree.GetProofForLeafAtIndex(forLeaf, untilLeaf)
					require.NoError(t, err)

					actualProof, err := merkleTree.GetProofForLeafAtIndex(ctx, forLeaf, untilLeaf)
					require.NoError(t, err)

					for i, expectedHash := range expectedProof {
						assert.EqualValues(t, expectedHash, actualProof[i])
					}

					rootNode, err := merkleTree.GetRootNodeForLeaf(ctx, untilLeaf)
					require.NoError(t, err)

					leafValue := fmt.Sprintf("leaf%d", forLeaf)
					assert.True(t, reference.Verify(expectedProof, reference.Hash(rootNode.Hash), reference.Leaf(leafValue)))
					assert.True(t, merkletree.Verify(actualProof, rootNode.Hash, merkletree.Leaf(leafValue)))
				}
			}
		})
	}
}

func assertEquivalentMetadata(t *testing.T, obj1, obj2 *merkletree.Metadata) {
	assert.Equal(t, obj1.Name, obj2.Name)
	assert.Equal(t, obj1.Levels, obj2.Levels)
	assert.Equal(t, obj1.NextIndex, obj2.NextIndex)
	assert.EqualValues(t, obj1.Seed, obj2.Seed)
}
