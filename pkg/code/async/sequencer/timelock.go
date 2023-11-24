package async_sequencer

import (
	"context"

	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

// The faster we can update timelock state, the better it is to unblock scheduling.
// Particularly, we don't want a missed Geyser account update to block scheduling.
// Generally, these are very safe if they're used when we first create and close
// accounts, because they denote the initial and end states.

func markTimelockLocked(ctx context.Context, data code_data.Provider, vault string, slot uint64) error {
	record, err := data.GetTimelockByVault(ctx, vault)
	if err != nil {
		return err
	}

	record.VaultState = timelock_token_v1.StateLocked
	record.Block = slot

	err = data.SaveTimelock(ctx, record)
	if err == timelock.ErrStaleTimelockState {
		return nil
	}
	return err
}

func markTimelockClosed(ctx context.Context, data code_data.Provider, vault string, slot uint64) error {
	record, err := data.GetTimelockByVault(ctx, vault)
	if err != nil {
		return err
	}

	record.VaultState = timelock_token_v1.StateClosed
	record.Block = slot

	err = data.SaveTimelock(ctx, record)
	if err == timelock.ErrStaleTimelockState {
		return nil
	}
	return err
}
