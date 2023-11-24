package async_commitment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
)

func TestCommitmentWorker_StateReadyToOpen_RemainInState_PrivacyUpgraded(t *testing.T) {
	env := setup(t)
	env.generateAvailableNonces(t, 10)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentVaultManagementFulfillmentsNotInjected(t, commitmentRecord)
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToOpen)

	// To trigger privacy deadline being met
	env.simulateSourceAccountUnlocked(t, commitmentRecord)

	newerCommitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	require.NoError(t, env.data.SaveCommitment(env.ctx, newerCommitmentRecord))
	commitmentRecord.RepaymentDivertedTo = &newerCommitmentRecord.Vault
	require.NoError(t, env.data.SaveCommitment(env.ctx, commitmentRecord))

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentVaultManagementFulfillmentsNotInjected(t, commitmentRecord)
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToOpen)
}

func TestCommitmentWorker_StateReadyToOpen_TransitionToStateOpening_TemporaryPrivacyDeadline(t *testing.T) {
	env := setup(t)
	env.generateAvailableNonces(t, 10)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentVaultManagementFulfillmentsNotInjected(t, commitmentRecord)
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToOpen)

	env.simulateSourceAccountUnlocked(t, commitmentRecord)

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpening)
	env.assertCommitmentVaultManagementFulfillmentsInjected(t, commitmentRecord)
}

func TestCommitmentWorker_StateReadyToOpen_TransitionToStateOpening_TargetForPermanentPrivacyCheques(t *testing.T) {
	env := setup(t)
	env.generateAvailableNonces(t, 10)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToOpen)
	env.assertCommitmentVaultManagementFulfillmentsNotInjected(t, commitmentRecord)

	upgradedCommitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	upgradedCommitmentRecord.RepaymentDivertedTo = &commitmentRecord.Vault
	require.NoError(t, env.data.SaveCommitment(env.ctx, upgradedCommitmentRecord))

	require.NoError(t, env.worker.handleReadyToOpen(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpening)
	env.assertCommitmentVaultManagementFulfillmentsInjected(t, commitmentRecord)
}

func TestCommitmentWorker_StateReadyToOpen_TransitionToStateReadyToRemoveFromMerkleTree(t *testing.T) {
	env := setup(t)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)

	require.NoError(t, env.worker.handleClosed(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToOpen)

	commitmentRecord.TreasuryRepaid = true
	require.NoError(t, env.data.SaveCommitment(env.ctx, commitmentRecord))

	require.NoError(t, env.worker.handleClosed(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToRemoveFromMerkleTree)
}

func TestCommitmentWorker_StateReadyToOpen_MarkTreasuryAsRepaid(t *testing.T) {
	for _, state := range []commitment.State{
		commitment.StateReadyToOpen,
		commitment.StateOpening,
		commitment.StateOpen,
		commitment.StateClosing,
		commitment.StateClosed,
		commitment.StateReadyToRemoveFromMerkleTree,
		commitment.StateRemovedFromMerkleTree,
	} {
		env := setup(t)

		upgradedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
		divertedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)

		env.simulateCommitmentBeingUpgraded(t, upgradedCommitment, divertedCommitment)

		divertedCommitment.State = state
		require.NoError(t, env.data.SaveCommitment(env.ctx, divertedCommitment))

		require.NoError(t, env.worker.handleReadyToOpen(env.ctx, upgradedCommitment))
		env.assertCommitmentRepaymentStatus(t, upgradedCommitment.Address, state >= commitment.StateClosed)
		env.assertCommitmentState(t, upgradedCommitment.Address, commitment.StateReadyToOpen)
	}
}

func TestCommitmentWorker_StateOpen_TransitionToStateClosing_TargetForPermanentPrivacyCheques(t *testing.T) {
	for _, fulfillmentState := range []fulfillment.State{
		fulfillment.StateConfirmed,
		fulfillment.StateFailed,
	} {
		env := setup(t)
		env.generateAvailableNonces(t, 10)

		divertedCommitmentRecords := env.simulateCommitments(t, 10, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
		env.simulateAddingLeaves(t, divertedCommitmentRecords)

		commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateOpen)
		env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})
		require.NoError(t, env.worker.injectCommitmentVaultManagementFulfillments(env.ctx, commitmentRecord))

		futureCommitmentRecords := env.simulateCommitments(t, 10, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
		env.simulateAddingLeaves(t, futureCommitmentRecords)

		for _, divertedCommitmentRecord := range divertedCommitmentRecords {
			env.simulateCommitmentBeingUpgraded(t, divertedCommitmentRecord, commitmentRecord)
		}
		env.simulateCommitmentBeingUpgraded(t, commitmentRecord, futureCommitmentRecords[0])

		for _, divertedCommitmentRecord := range divertedCommitmentRecords {
			env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpen)
			env.simulatePermanentPrivacyChequeCashed(t, divertedCommitmentRecord, fulfillmentState)
			require.NoError(t, env.worker.handleOpen(env.ctx, commitmentRecord))
		}

		if fulfillmentState == fulfillment.StateConfirmed {
			env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateClosing)
		} else {
			env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpen)
		}
	}
}

func TestCommitmentWorker_StateOpen_TransitionToStateClosing_CashTemporaryPrivacyCheque(t *testing.T) {
	for _, fulfillmentState := range []fulfillment.State{
		fulfillment.StateConfirmed,
		fulfillment.StateFailed,
	} {
		env := setup(t)
		env.generateAvailableNonces(t, 10)

		commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateOpen)
		env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})
		require.NoError(t, env.worker.injectCommitmentVaultManagementFulfillments(env.ctx, commitmentRecord))

		futureCommitmentRecords := env.simulateCommitments(t, 10, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
		env.simulateAddingLeaves(t, futureCommitmentRecords)

		require.NoError(t, env.worker.handleOpen(env.ctx, commitmentRecord))
		env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpen)

		env.simulateTemporaryPrivacyChequeCashed(t, commitmentRecord, fulfillmentState)

		require.NoError(t, env.worker.handleOpen(env.ctx, commitmentRecord))
		if fulfillmentState == fulfillment.StateConfirmed {
			env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateClosing)
		} else {
			env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpen)
		}
	}
}

func TestCommitmentWorker_StateOpen_TransitionToStateClosing_PrivacyUpgradeCandidateTimeout(t *testing.T) {
	env := setup(t)
	env.generateAvailableNonces(t, 10)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateOpen)
	env.simulateAddingLeaves(t, []*commitment.Record{commitmentRecord})
	require.NoError(t, env.worker.injectCommitmentVaultManagementFulfillments(env.ctx, commitmentRecord))
	env.simulateTemporaryPrivacyChequeCashed(t, commitmentRecord, fulfillment.StateConfirmed)

	otherCommitmentRecords := env.simulateCommitments(t, 10, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	env.simulateAddingLeaves(t, otherCommitmentRecords)

	privacyUpgradeCandidateSelectionTimeout = 100 * time.Millisecond
	require.NoError(t, env.worker.handleOpen(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateOpen)

	time.Sleep(2 * privacyUpgradeCandidateSelectionTimeout)
	require.NoError(t, env.worker.handleOpen(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateClosing)
}

func TestCommitmentWorker_StateOpen_MarkTreasuryAsRepaid(t *testing.T) {
	for _, state := range []commitment.State{
		commitment.StateReadyToOpen,
		commitment.StateOpening,
		commitment.StateOpen,
		commitment.StateClosing,
		commitment.StateClosed,
		commitment.StateReadyToRemoveFromMerkleTree,
		commitment.StateRemovedFromMerkleTree,
	} {
		env := setup(t)

		upgradedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateOpen)
		divertedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)

		env.simulateCommitmentBeingUpgraded(t, upgradedCommitment, divertedCommitment)

		divertedCommitment.State = state
		require.NoError(t, env.data.SaveCommitment(env.ctx, divertedCommitment))

		require.NoError(t, env.worker.handleReadyToOpen(env.ctx, upgradedCommitment))
		env.assertCommitmentRepaymentStatus(t, upgradedCommitment.Address, state >= commitment.StateClosed)
		env.assertCommitmentState(t, upgradedCommitment.Address, commitment.StateOpen)
	}
}

func TestCommitmentWorker_StateClosed_TransitionToStateReadyToRemoveFromMerkleTree(t *testing.T) {
	env := setup(t)

	commitmentRecord := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateClosed)

	require.NoError(t, env.worker.handleClosed(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateClosed)

	commitmentRecord.TreasuryRepaid = true
	require.NoError(t, env.data.SaveCommitment(env.ctx, commitmentRecord))

	require.NoError(t, env.worker.handleClosed(env.ctx, commitmentRecord))
	env.assertCommitmentState(t, commitmentRecord.Address, commitment.StateReadyToRemoveFromMerkleTree)
}

func TestCommitmentWorker_StateClosed_MarkTreasuryAsRepaid(t *testing.T) {
	for _, state := range []commitment.State{
		commitment.StateReadyToOpen,
		commitment.StateOpening,
		commitment.StateOpen,
		commitment.StateClosing,
		commitment.StateClosed,
		commitment.StateReadyToRemoveFromMerkleTree,
		commitment.StateRemovedFromMerkleTree,
	} {
		env := setup(t)

		upgradedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateClosed)
		divertedCommitment := env.simulateCommitment(t, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)

		env.simulateCommitmentBeingUpgraded(t, upgradedCommitment, divertedCommitment)

		divertedCommitment.State = state
		require.NoError(t, env.data.SaveCommitment(env.ctx, divertedCommitment))

		require.NoError(t, env.worker.handleClosed(env.ctx, upgradedCommitment))
		env.assertCommitmentRepaymentStatus(t, upgradedCommitment.Address, state >= commitment.StateClosed)
		env.assertCommitmentState(t, upgradedCommitment.Address, commitment.StateClosed)
	}
}
