package transaction_v2

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	commitment_worker "github.com/code-payments/code-server/pkg/code/async/commitment"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

type refreshingMerkleTree struct {
	tree            *merkletree.MerkleTree
	lastRefreshedAt time.Time
}

var (
	merkleTreeLock    sync.Mutex
	cachedMerkleTrees map[string]*refreshingMerkleTree
)

func init() {
	cachedMerkleTrees = make(map[string]*refreshingMerkleTree)
}

type privacyUpgradeProof struct {
	proof []merkletree.Hash

	newCommitment            *common.Account
	newCommitmentVault       *common.Account
	newCommitmentTranscript  merkletree.Hash
	newCommitmentDestination *common.Account
	newCommitmentAmount      uint64
	newCommitmentRoot        merkletree.Hash
}

type privacyUpgradeCandidate struct {
	newCommitmentRecord *commitment.Record

	forLeafHash merkletree.Hash
	forLeaf     uint64

	recentRoot merkletree.Hash
	untilLeaf  uint64
}

func canUpgradeCommitmentAction(ctx context.Context, data code_data.Provider, commitmentRecord *commitment.Record) (bool, error) {
	_, err := selectCandidateForPrivacyUpgrade(ctx, data, commitmentRecord.Intent, commitmentRecord.ActionId)
	switch err {
	case ErrPrivacyAlreadyUpgraded, ErrWaitForNextBlock, ErrPrivacyUpgradeMissed:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

// Note: How we get select commmitments for proofs plays into how we decide to
// close commitment vaults. Updates to logic should be in sync.
func selectCandidateForPrivacyUpgrade(ctx context.Context, data code_data.Provider, intentId string, actionId uint32) (*privacyUpgradeCandidate, error) {
	actionRecord, err := data.GetActionById(ctx, intentId, actionId)
	if err != nil {
		return nil, err
	}
	if actionRecord.ActionType != action.PrivateTransfer {
		return nil, ErrInvalidActionToUpgrade
	}

	switch actionRecord.State {
	case action.StateUnknown:
		// Action isn't scheduled, so no fulfillments were either. Keep waiting.
		return nil, ErrWaitForNextBlock
	case action.StateRevoked:
		// Aciton is revoked, so it can never be upgraded.
		return nil, ErrInvalidActionToUpgrade
	case action.StateFailed, action.StateConfirmed:
		// Because commitmentRecord.RepaymentDivertedTo is nil, we must have submitted
		// the temporary private transfer.
		return nil, ErrPrivacyUpgradeMissed
	}

	commitmentRecord, err := data.GetCommitmentByAction(ctx, intentId, actionId)
	if err != nil {
		return nil, err
	}
	if commitmentRecord.RepaymentDivertedTo != nil {
		// The private payment has already been upgraded
		return nil, ErrPrivacyAlreadyUpgraded
	}

	privacyUpgradeDeadline, err := commitment_worker.GetDeadlineToUpgradePrivacy(ctx, data, commitmentRecord)
	switch err {
	case nil:
		// We're too close to the privacy upgrade deadline, so mark it as missed
		// to avoid any kind of races.
		if privacyUpgradeDeadline.Add(-5 * time.Minute).Before(time.Now()) {
			return nil, ErrPrivacyUpgradeMissed
		}
	case commitment_worker.ErrNoPrivacyUpgradeDeadline:
	default:
		return nil, err
	}

	merkleTree, err := getCachedMerkleTreeForTreasury(ctx, data, commitmentRecord.Pool)
	if err != nil {
		return nil, err
	}

	commitmentAddressBytes, err := base58.Decode(commitmentRecord.Address)
	if err != nil {
		return nil, err
	}

	commitmentLeafNode, err := merkleTree.GetLeafNode(ctx, commitmentAddressBytes)
	if err == merkletree.ErrLeafNotFound {
		// The commitment isn't in the merkle tree, so we need to wait to observe it
		// added as a leaf from the blockchain state.
		return nil, ErrWaitForNextBlock
	} else if err != nil {
		return nil, err
	}

	// Note: Assumes we've coded SubmitIntent and the scheduling layer to ensure
	// we create new commitments with an ever-progressing recent root that is the
	// latest value.
	latestLeafNode, err := merkleTree.GetLastAddedLeafNode(ctx)
	if err == merkletree.ErrLeafNotFound {
		// The merkle tree is empty, which means we're starting off with a fresh
		// treasury pool and need to see further state before we can do anything.
		return nil, ErrWaitForNextBlock
	} else if err != nil {
		return nil, err
	}

	latestCommitmentRecord, err := data.GetCommitmentByAddress(ctx, base58.Encode(latestLeafNode.LeafValue))
	if err != nil {
		return nil, err
	}

	recentRootForProof, err := hex.DecodeString(latestCommitmentRecord.RecentRoot)
	if err != nil {
		return nil, err
	}

	recentRootLeafNode, err := merkleTree.GetLeafNodeForRoot(ctx, recentRootForProof)
	if err == merkletree.ErrLeafNotFound || err == merkletree.ErrRootNotFound {
		// The recent root isn't in the merkle tree, which likely means we're
		// starting off with a new treasury pool and haven't observed sufficient
		// state.
		return nil, ErrWaitForNextBlock
	} else if err != nil {
		return nil, err
	}

	if commitmentLeafNode.Index > recentRootLeafNode.Index {
		// The commitment happened on or after the recent root, so we need to wait
		// and observe another.
		return nil, ErrWaitForNextBlock
	}

	return &privacyUpgradeCandidate{
		newCommitmentRecord: latestCommitmentRecord,

		forLeafHash: commitmentAddressBytes,
		forLeaf:     commitmentLeafNode.Index,

		recentRoot: recentRootForProof,
		untilLeaf:  recentRootLeafNode.Index,
	}, nil
}

// Note: How we get proofs plays into how we decide to close commitment vaults. Updates to
// logic should be in sync.
func getProofForPrivacyUpgrade(ctx context.Context, data code_data.Provider, upgradingTo *privacyUpgradeCandidate) (*privacyUpgradeProof, error) {
	merkleTree, err := getCachedMerkleTreeForTreasury(ctx, data, upgradingTo.newCommitmentRecord.Pool)
	if err != nil {
		return nil, err
	}

	proof, err := merkleTree.GetProofForLeafAtIndex(ctx, upgradingTo.forLeaf, upgradingTo.untilLeaf)
	if err != nil {
		return nil, err
	}

	// Keeping this in as a sanity check, for now. If we can't validate the proof,
	// then the client certainly can't either.
	if !merkletree.Verify(proof, upgradingTo.recentRoot, merkletree.Leaf(upgradingTo.forLeafHash)) {
		return nil, errors.New("proof unexpectedly failed verification")
	}

	newCommitmentAccount, err := common.NewAccountFromPublicKeyString(upgradingTo.newCommitmentRecord.Address)
	if err != nil {
		return nil, err
	}

	newCommitmentVaultAccount, err := common.NewAccountFromPublicKeyString(upgradingTo.newCommitmentRecord.VaultAddress)
	if err != nil {
		return nil, err
	}

	newCommitmentDestinationAccount, err := common.NewAccountFromPublicKeyString(upgradingTo.newCommitmentRecord.Destination)
	if err != nil {
		return nil, err
	}

	newCommitmentTranscript, err := hex.DecodeString(upgradingTo.newCommitmentRecord.Transcript)
	if err != nil {
		return nil, err
	}

	return &privacyUpgradeProof{
		proof: proof,

		newCommitment:            newCommitmentAccount,
		newCommitmentVault:       newCommitmentVaultAccount,
		newCommitmentDestination: newCommitmentDestinationAccount,
		newCommitmentTranscript:  newCommitmentTranscript,
		newCommitmentAmount:      upgradingTo.newCommitmentRecord.Amount,
		newCommitmentRoot:        upgradingTo.recentRoot,
	}, nil
}

// todo: move this into a common spot? code is duplicated
func getCachedMerkleTreeForTreasury(ctx context.Context, data code_data.Provider, address string) (*merkletree.MerkleTree, error) {
	merkleTreeLock.Lock()
	defer merkleTreeLock.Unlock()

	cachedTreasuryMetadata, err := getCachedTreasuryMetadataByNameOrAddress(ctx, data, address, 7*31*24*time.Hour)
	if err != nil {
		return nil, err
	}
	name := cachedTreasuryMetadata.name

	cached, ok := cachedMerkleTrees[name]
	if !ok {
		loaded, err := data.LoadExistingMerkleTree(ctx, name, true)
		if err != nil {
			return nil, err
		}

		cached = &refreshingMerkleTree{
			tree:            loaded,
			lastRefreshedAt: time.Now(),
		}
		cachedMerkleTrees[name] = cached
	}

	// Refresh the merkle tree periodically, instead of loading it every time.
	//
	// todo: configurable value (put an upper bound when it does become configurable)
	// todo: keep this small relative to the commitment worker's close timeout threshold (currently 10m) for the next leaf (until we have distributed locks at least)
	if time.Since(cached.lastRefreshedAt) > time.Minute {
		err := cached.tree.Refresh(ctx)
		if err != nil {
			return nil, err
		}

		cached.lastRefreshedAt = time.Now()
	}

	return cached.tree, nil
}
