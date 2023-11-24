package async_commitment

import (
	"context"
	"sync"
	"time"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

type refreshingMerkleTree struct {
	tree            *merkletree.MerkleTree
	lastRefreshedAt time.Time
}

var (
	merkleTreeLock            sync.Mutex
	treasuryPoolAddressToName map[string]string
	cachedMerkleTrees         map[string]*refreshingMerkleTree
)

func init() {
	treasuryPoolAddressToName = make(map[string]string)
	cachedMerkleTrees = make(map[string]*refreshingMerkleTree)
}

// todo: move this into a common spot? code is duplicated
func getCachedMerkleTreeForTreasury(ctx context.Context, data code_data.Provider, address string) (*merkletree.MerkleTree, error) {
	merkleTreeLock.Lock()
	defer merkleTreeLock.Unlock()

	name, ok := treasuryPoolAddressToName[address]
	if !ok {
		treasuryPoolRecord, err := data.GetTreasuryPoolByAddress(ctx, address)
		if err != nil {
			return nil, err
		}
		name = treasuryPoolRecord.Name
		treasuryPoolAddressToName[address] = name
	}

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
	if time.Since(cached.lastRefreshedAt) > time.Minute {
		err := cached.tree.Refresh(ctx)
		if err != nil {
			return nil, err
		}

		cached.lastRefreshedAt = time.Now()
	}

	return cached.tree, nil
}
