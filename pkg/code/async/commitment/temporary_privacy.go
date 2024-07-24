package async_commitment

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

var (
	// todo: bump this significantly after clients implement upgrades
	// todo: better configuration mechanism
	privacyUpgradeTimeout = 15 * time.Minute

	privacyUpgradeTimeoutCache = cache.NewCache(1_000_000)
)

// GetDeadlineToUpgradePrivacy figures out at what point in time the temporary
// private transfer to the commitment should be played out. If no deadline
// exists, then ErrNoPrivacyUpgradeDeadline is returned.
//
// todo: move this someplace more common?
func GetDeadlineToUpgradePrivacy(ctx context.Context, data code_data.Provider, commitmentRecord *commitment.Record) (*time.Time, error) {
	if commitmentRecord.RepaymentDivertedTo != nil {
		// There is no deadline because the privacy upgrade has already occurred
		return nil, ErrNoPrivacyUpgradeDeadline
	}

	if commitmentRecord.TreasuryRepaid {
		// There is no deadline because the temporary private transfer has already
		// been cashed in.
		return nil, ErrNoPrivacyUpgradeDeadline
	}

	actionRecord, err := data.GetActionById(ctx, commitmentRecord.Intent, commitmentRecord.ActionId)
	if err != nil {
		return nil, err
	}

	timelockRecord, err := data.GetTimelockByVault(ctx, actionRecord.Source)
	if err != nil {
		return nil, err
	}

	if !common.IsManagedByCode(ctx, timelockRecord) {
		// The source account is no longer managed by Code. We need to cash this
		// in yesterday.
		t := time.Now().Add(-time.Hour)
		return &t, nil
	}

	cached, ok := privacyUpgradeTimeoutCache.Retrieve(commitmentRecord.Address)
	if ok {
		cloned := cached.(time.Time)
		return &cloned, nil
	}

	merkleTree, err := getCachedMerkleTreeForTreasury(ctx, data, commitmentRecord.Pool)
	if err == merkletree.ErrMerkleTreeNotFound {
		// Merkle tree doesn't exist, so we're likely operating on a new treasury
		// pool
		return nil, ErrNoPrivacyUpgradeDeadline
	} else if err != nil {
		return nil, err
	}

	commitmentAccount, err := common.NewAccountFromPublicKeyString(commitmentRecord.Address)
	if err != nil {
		return nil, err
	}

	leafNode, err := merkleTree.GetLeafNode(ctx, commitmentAccount.PublicKey().ToBytes())
	if err == merkletree.ErrLeafNotFound {
		// The commitment hasn't even been observed by our local merkle tree,
		// so keep pushing back the deadline. There's nothing we can do because
		// the account can't even be opened without a proof, let alone have a
		// proof for a client upgrade.
		return nil, ErrNoPrivacyUpgradeDeadline
	} else if err != nil {
		return nil, err
	}

	t := leafNode.CreatedAt.Add(privacyUpgradeTimeout)
	cloned := t
	privacyUpgradeTimeoutCache.Insert(commitmentRecord.Address, cloned, 1)
	return &t, nil
}
