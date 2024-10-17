package async_commitment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
)

func TestGetDeadlineToUpgradePrivacy_HappyPath(t *testing.T) {
	env := setup(t)

	commitmentRecords := env.simulateCommitments(t, 2, env.treasuryPool.GetMostRecentRoot(), commitment.StateReadyToOpen)
	commitmentRecord := commitmentRecords[0]

	// Commitment isn't in the merkle tree
	_, err := GetDeadlineToUpgradePrivacy(env.ctx, env.data, commitmentRecord)
	assert.Equal(t, ErrNoPrivacyUpgradeDeadline, err)

	env.simulateAddingLeaves(t, commitmentRecords)

	leafNode, err := env.merkleTree.GetLeafNodeByIndex(env.ctx, 0)
	require.NoError(t, err)

	// Commitment is in the merkle tree and deadline is based on leaf creation timestamp
	actual, err := GetDeadlineToUpgradePrivacy(env.ctx, env.data, commitmentRecord)
	require.NoError(t, err)
	assert.Equal(t, leafNode.CreatedAt.Add(privacyUpgradeTimeout), *actual)

	// Treasury is repaid, so privacy can't be upgraded
	commitmentRecord.TreasuryRepaid = true
	_, err = GetDeadlineToUpgradePrivacy(env.ctx, env.data, commitmentRecord)
	assert.Equal(t, ErrNoPrivacyUpgradeDeadline, err)
	commitmentRecord.TreasuryRepaid = false

	// Privacy is already upgraded
	commitmentRecord.RepaymentDivertedTo = &commitmentRecords[1].VaultAddress
	_, err = GetDeadlineToUpgradePrivacy(env.ctx, env.data, commitmentRecord)
	assert.Equal(t, ErrNoPrivacyUpgradeDeadline, err)
	commitmentRecord.RepaymentDivertedTo = nil

	// Source user account has been unlocked
	env.simulateSourceAccountUnlocked(t, commitmentRecord)
	actual, err = GetDeadlineToUpgradePrivacy(env.ctx, env.data, commitmentRecord)
	require.NoError(t, err)
	assert.True(t, actual.Before(time.Now()))
}
