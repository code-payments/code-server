package async_geyser

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

func updateTimelockAccountRecord(ctx context.Context, data code_data.Provider, timelockRecord *timelock.Record) error {
	// Wait for Timelock account initialization before monitoring state
	// to avoid conflicting with the sequencer
	if timelockRecord.VaultState == timelock_token.StateUnknown || timelockRecord.Block == 0 {
		return nil
	}

	unlockState, slot, err := getTimelockUnlockState(ctx, data, timelockRecord)
	if err != nil {
		return err
	}

	if unlockState != nil {
		timelockRecord.VaultState = timelock_token.StateWaitingForTimeout
		if unlockState.IsUnlocked() {
			timelockRecord.VaultState = timelock_token.StateUnlocked
		}

		unlockAt := uint64(unlockState.UnlockAt)
		timelockRecord.UnlockAt = &unlockAt
	}
	timelockRecord.Block = slot
	timelockRecord.LastUpdatedAt = time.Now()

	return data.SaveTimelock(ctx, timelockRecord)
}

func getTimelockUnlockState(ctx context.Context, data code_data.Provider, timelockRecord *timelock.Record) (*cvm.UnlockStateAccount, uint64, error) {
	ownerAccount, err := common.NewAccountFromPublicKeyString(timelockRecord.VaultOwner)
	if err != nil {
		return nil, 0, err
	}

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
	if err != nil {
		return nil, 0, err
	}

	marshalled, slot, err := data.GetBlockchainAccountDataAfterBlock(ctx, timelockAccounts.Unlock.PublicKey().ToBase58(), timelockRecord.Block)
	switch err {
	case nil:
		var unlockState cvm.UnlockStateAccount
		if err = unlockState.Unmarshal(marshalled); err != nil {
			return nil, 0, err
		}
		return &unlockState, slot, nil
	case solana.ErrNoAccountInfo:
		return nil, slot, nil
	default:
		return nil, 0, err
	}
}
