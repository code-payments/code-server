package async_geyser

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
	"github.com/code-payments/code-server/pkg/solana"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

const (
	secondsPerDay              = 86400 // 60sec * 60min * 24hrs = 86400
	timelockV1UnlockTimeOffset = 172
)

func findUnlockedTimelockV1Accounts(ctx context.Context, data code_data.Provider, daysFromToday uint8) ([]string, uint64, error) {
	ts := time.Now().Unix() + int64(daysFromToday)*secondsPerDay

	// Normalize the unlock time to the start of the UTC day. Source reference:
	// https://github.com/code-payments/code-program-library/blob/901296f86ee9202408001cb57abc5218cecf2457/timelock-token/programs/timelock-token/src/lib.rs#L86-L93
	tsAtStartofUtc := ts
	if ts%secondsPerDay != 0 {
		tsAtStartofUtc = ts + (secondsPerDay - (ts % secondsPerDay))
	}

	dataFilterValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(dataFilterValue, uint64(tsAtStartofUtc))

	var addresses []string
	var slot uint64
	var err error
	_, err = retry.Retry(
		func() error {
			addresses, slot, err = data.GetBlockchainFilteredProgramAccounts(
				ctx,
				base58.Encode(timelock_token_v1.PROGRAM_ID),
				timelockV1UnlockTimeOffset,
				dataFilterValue,
			)
			return err
		},
		retry.NonRetriableErrors(context.Canceled),
		retry.Limit(3),
		retry.Backoff(backoff.BinaryExponential(time.Second), 5*time.Second),
	)
	if err != nil {
		return nil, 0, errors.Wrap(err, "error getting filtered timelock program accounts")
	}
	return addresses, slot, nil
}

func updateTimelockV1AccountCachedState(ctx context.Context, data code_data.Provider, stateAccount *common.Account, minSlot uint64) error {
	timelockRecord, err := data.GetTimelockByAddress(ctx, stateAccount.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		// Not a timelock we care about
		return nil
	} else if err != nil {
		return errors.Wrap(err, "error getting timelock record")
	}

	var finalizedData []byte
	var finalizedSlot uint64
	_, err = retry.Retry(
		func() error {
			finalizedData, finalizedSlot, err = data.GetBlockchainAccountDataAfterBlock(ctx, stateAccount.PublicKey().ToBase58(), minSlot)
			return err
		},
		append(
			[]retry.Strategy{
				retry.NonRetriableErrors(solana.ErrNoAccountInfo),
			},
			waitForFinalizationRetryStrategies...,
		)...,
	)
	if err != nil && err != solana.ErrNoAccountInfo {
		return errors.Wrap(err, "error getting finalized account data")
	}

	if err == solana.ErrNoAccountInfo {
		timelockRecord.VaultState = timelock_token_v1.StateClosed
		timelockRecord.Block = finalizedSlot
	} else {
		var finalizedState timelock_token_v1.TimelockAccount
		err = finalizedState.Unmarshal(finalizedData)
		if err != nil {
			return errors.Wrap(err, "error unmarshalling account data from finalized data")
		}

		err = timelockRecord.UpdateFromV1ProgramAccount(&finalizedState, finalizedSlot)
		if err == timelock.ErrStaleTimelockState {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "error updating timelock record locally")
		}
	}

	err = data.SaveTimelock(ctx, timelockRecord)
	if err != nil && err != timelock.ErrStaleTimelockState {
		return errors.Wrap(err, "error saving timelock record")
	}
	return nil
}
