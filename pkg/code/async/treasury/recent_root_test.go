package async_treasury

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

func TestMaybeSaveRecentRoot_HappyPath(t *testing.T) {
	for _, allowSecondSave := range []bool{true, false} {
		expectedMinAdvances := 180

		env := setup(t, &testOverrides{
			hideInTheCrowdPrivacyLevel: 10,
		})
		env.generateAvailableNonces(t, 2)

		require.EqualValues(t, expectedMinAdvances, getMinTransactionsForBucketedPrivacyLevel(10))

		// No collected treasury advances
		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentNotCreated(t)

		// Too few collected treasury advances
		commitmentRecords := env.simulateCommitments(t, expectedMinAdvances/2, env.treasuryPool.GetMostRecentRoot(), commitment.StateUnknown)

		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentNotCreated(t)

		env.assertTreasuryAdvancesActiveSchedulingState(t, commitmentRecords, false)

		// Sufficient terasury advances collected
		commitmentRecords = append(
			commitmentRecords,
			env.simulateCommitments(t, expectedMinAdvances/2, env.treasuryPool.GetMostRecentRoot(), commitment.StateUnknown)...,
		)

		if allowSecondSave {
			// The most recent root value used shouldn't affect decision making.
			// Notably, we won't include these commitments in the simulated worker
			// flows so these commitments actually get applied for the second save.
			env.simulateCommitments(t, expectedMinAdvances, env.treasuryPool.GetMostRecentRoot(), commitment.StateUnknown)
		}

		// Recent root is now saved
		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentCount(t, 1)
		env.assertIntentCreated(t)

		env.assertTreasuryAdvancesActiveSchedulingState(t, commitmentRecords, true)

		// Simulate advances to the blockchain
		env.simulateConfirmedAdvances(t, commitmentRecords)

		// Previous intent isn't confirmed
		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentCount(t, 1)

		env.simulateConfirmedIntent(t)

		// Local treasury pool view hasn't been updated
		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentCount(t, 1)

		nextRecentRoot := env.getNextRecentRoot(t, commitmentRecords)
		env.simulateTreasuryPoolUpdated(t, commitmentRecords)
		require.Equal(t, nextRecentRoot, env.treasuryPool.GetMostRecentRoot())

		// Local merkle tree view hasn't been updated
		require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
		env.assertIntentCount(t, 1)

		env.simulateAddingLeaves(t, commitmentRecords)

		if allowSecondSave {
			// Recent root can now be saved because sufficiently more advances have been collected
			require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
			env.assertIntentCount(t, 2)
			env.assertIntentCreated(t)
		} else {
			// Recent root cannot be saved because advance collection hasn't progressed enough
			require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
			env.assertIntentCount(t, 1)
		}
	}
}

func TestMaybeSaveRecentRoot_TimeoutWindow(t *testing.T) {
	advanceCollectionTimeout := time.Second / 4

	env := setup(t, &testOverrides{advanceCollectionTimeout: advanceCollectionTimeout})
	env.generateAvailableNonces(t, 2)

	commitmentRecords := env.simulateCommitments(t, 1, env.treasuryPool.GetMostRecentRoot(), commitment.StateUnknown)

	// Timeout not met for brand new treasury
	require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
	env.assertIntentNotCreated(t)

	time.Sleep(advanceCollectionTimeout)

	// Timeout met for brand new treasury
	require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
	env.assertIntentCount(t, 1)
	env.assertIntentCreated(t)

	env.simulateConfirmedAdvances(t, commitmentRecords)
	env.simulateConfirmedIntent(t)
	env.simulateTreasuryPoolUpdated(t, commitmentRecords)
	env.simulateAddingLeaves(t, commitmentRecords)

	commitmentRecords = env.simulateCommitments(t, 1, env.treasuryPool.GetMostRecentRoot(), commitment.StateUnknown)

	// Timeout not met since last saved recent root
	require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
	env.assertIntentCount(t, 1)

	time.Sleep(advanceCollectionTimeout)

	// Timeout met since last saved recent root
	require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
	env.assertIntentCount(t, 2)
	env.assertIntentCreated(t)

	env.simulateConfirmedAdvances(t, commitmentRecords)
	env.simulateConfirmedIntent(t)
	env.simulateTreasuryPoolUpdated(t, commitmentRecords)
	env.simulateAddingLeaves(t, commitmentRecords)

	time.Sleep(advanceCollectionTimeout)

	// Recent root isn't created if there are no advances and the timeout is met
	require.NoError(t, env.worker.maybeSaveRecentRoot(env.ctx, env.treasuryPool))
	env.assertIntentCount(t, 2)
}
