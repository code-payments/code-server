package async_treasury

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/mr-tron/base58"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// todo: Add tests for syncMerkleTree for treasury payments that land on the
//       same block, which requires a mocked Solana client that doesn't currently
//       exist.

func TestSyncMerkleTree(t *testing.T) {
	env := setup(t, &testOverrides{})

	// Nothing happens when there are no values to sync
	require.NoError(t, env.worker.syncMerkleTree(env.ctx, env.treasuryPool))
	env.assertEmptyMerkleTree(t)

	// Empty merkle tree (no starting checkpoint, but an endpoint checkpoint exists)
	allCommitmentRecords := env.simulateCommitments(t, 100, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	env.simulateMostRecentRoot(t, intent.StateConfirmed, allCommitmentRecords)
	require.NoError(t, env.worker.syncMerkleTree(env.ctx, env.treasuryPool))
	env.assertSyncedMerkleTree(t, allCommitmentRecords)

	// Merkle tree with some leaves (both starting and ending checkpoint exist) over
	// a few iterations
	for i := 0; i < 10; i++ {
		newCommitmentRecords := env.simulateCommitments(t, rand.Intn(100)+1, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
		env.simulateMostRecentRoot(t, intent.StatePending, newCommitmentRecords)

		// Intent isn't confirmed, so we error out
		assert.Error(t, env.worker.syncMerkleTree(env.ctx, env.treasuryPool))

		// Initial sync, which should yeild an updated merkle tree
		env.simulateConfirmedIntent(t)
		require.NoError(t, env.worker.syncMerkleTree(env.ctx, env.treasuryPool))
		allCommitmentRecords = append(allCommitmentRecords, newCommitmentRecords...)
		env.assertSyncedMerkleTree(t, allCommitmentRecords)

		// Idempotency check where we shouldn't resync leaf values that have already
		// been added to the merkle tree
		require.NoError(t, env.worker.syncMerkleTree(env.ctx, env.treasuryPool))
		env.assertSyncedMerkleTree(t, allCommitmentRecords)
	}
}

func TestSafelyAddToMerkleTree(t *testing.T) {
	env := setup(t, &testOverrides{})

	commitmentRecords := env.simulateCommitments(t, 10, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)

	paymentRecords, err := env.data.GetPaymentHistory(env.ctx, env.treasuryPool.Vault)
	require.NoError(t, err)
	require.Len(t, paymentRecords, 10)

	// No leaves to add
	err = env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, nil)
	assert.True(t, strings.Contains(err.Error(), "no treasury payments to add to the merkle tree"))
	env.assertEmptyMerkleTree(t)

	// Treasury pool most recent root isn't available to simulate adding leaves
	err = env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, paymentRecords)
	assert.True(t, strings.Contains(err.Error(), "calculated an incorrect recent root"))
	env.assertEmptyMerkleTree(t)

	env.simulateMostRecentRoot(t, intent.StateConfirmed, commitmentRecords)

	// Payment records are out of order and won't lead to a successful simulation
	firstPaymentRecord := paymentRecords[0]
	paymentRecords[0] = paymentRecords[len(paymentRecords)-1]
	paymentRecords[len(paymentRecords)-1] = firstPaymentRecord
	err = env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, paymentRecords)
	assert.True(t, strings.Contains(err.Error(), "calculated an incorrect recent root"))
	env.assertEmptyMerkleTree(t)

	paymentRecords, err = env.data.GetPaymentHistory(env.ctx, env.treasuryPool.Vault)
	require.NoError(t, err)
	require.Len(t, paymentRecords, 10)

	// Subset of apyment records are missing and won't lead to a successful simulation
	paymentRecords = append(paymentRecords[:3], paymentRecords[7:]...)
	err = env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, paymentRecords)
	assert.True(t, strings.Contains(err.Error(), "calculated an incorrect recent root"))
	env.assertEmptyMerkleTree(t)

	paymentRecords, err = env.data.GetPaymentHistory(env.ctx, env.treasuryPool.Vault)
	require.NoError(t, err)
	require.Len(t, paymentRecords, 10)

	// Payment records are in the right order and will successfully lead to a correct simulation
	require.NoError(t, env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, paymentRecords))
	require.NoError(t, env.merkleTree.Refresh(env.ctx))
	for i, commitmentRecord := range commitmentRecords {
		expectedLeafValue, err := base58.Decode(commitmentRecord.Address)
		require.NoError(t, err)

		leafNode, err := env.merkleTree.GetLeafNodeByIndex(env.ctx, uint64(i))
		require.NoError(t, err)
		assert.EqualValues(t, expectedLeafValue, leafNode.LeafValue)
	}

	// Can't double add leaves
	err = env.worker.safelyAddToMerkleTree(env.ctx, env.treasuryPool, env.merkleTree, paymentRecords)
	assert.True(t, strings.Contains(err.Error(), "calculated an incorrect recent root"))
}
