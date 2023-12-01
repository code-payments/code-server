package async_geyser

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/cache"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/push"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/kin"
	push_lib "github.com/code-payments/code-server/pkg/push"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	codeMemoValue = "ZTAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
)

var (
	syncedDepositCache = cache.NewCache(1_000_000)
)

func fixMissingExternalDeposits(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, vault *common.Account) error {
	signatures, err := findPotentialExternalDeposits(ctx, data, vault)
	if err != nil {
		return errors.Wrap(err, "error finding potential external deposits")
	}

	var anyError error
	for _, signature := range signatures {
		err := processPotentialExternalDeposit(ctx, data, pusher, signature, vault)
		if err != nil {
			anyError = errors.Wrap(err, "error processing signature for external deposit")
		}
	}
	if anyError != nil {
		return anyError
	}

	return markDepositsAsSynced(ctx, data, vault)
}

// Note: This puts an upper bound to how far back in history we'll search
//
// todo: We can track the furthest succesful signature, so we have a bound.
// This would also enable us to not reprocess transactions. We'll need to be
// careful to check commitment status, since GetBlockchainHistory could return
// non-finalized transactions.
func findPotentialExternalDeposits(ctx context.Context, data code_data.Provider, vault *common.Account) ([]string, error) {
	var res []string
	var cursor []byte
	var totalTransactionsFound int
	for {

		history, err := data.GetBlockchainHistory(
			ctx,
			vault.PublicKey().ToBase58(),
			solana.CommitmentConfirmed, // Get signatures faster, which is ok because we'll fetch finalized txn data
			query.WithLimit(1_000),     // Max supported
			query.WithCursor(cursor),
		)
		if err != nil {
			return nil, errors.Wrap(err, "error getting signatures for address")
		}

		if len(history) == 0 {
			return res, nil
		}

		for _, historyItem := range history {
			// If there's a Code memo, then it isn't an external deposit
			if historyItem.Memo != nil && strings.Contains(*historyItem.Memo, codeMemoValue) {
				continue
			}

			// Transaction has an error, so we cannot add its funds
			if historyItem.Err != nil {
				continue
			}

			res = append(res, base58.Encode(historyItem.Signature[:]))

			// Bound total results
			if len(res) >= 100 {
				return res, nil
			}
		}

		// Bound total history to look for in the past
		totalTransactionsFound += len(history)
		if totalTransactionsFound >= 10_000 {
			return res, nil
		}

		cursor = query.Cursor(history[len(history)-1].Signature[:])
	}
}

func processPotentialExternalDeposit(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, signature string, vault *common.Account) error {
	decodedSignature, err := base58.Decode(signature)
	if err != nil {
		return errors.Wrap(err, "invalid signature")
	}
	var typedSignature solana.Signature
	copy(typedSignature[:], decodedSignature)

	// Avoid reprocessing deposits we've recently seen and processed. Particularly,
	// the backup process will likely be triggered in frequent bursts, so this is
	// just an optimization around that.
	cacheKey := getSyncedDepositCacheKey(signature, vault)
	_, ok := syncedDepositCache.Retrieve(cacheKey)
	if ok {
		return nil
	}

	// Is this transaction a fulfillment? If so, it cannot be an external deposit.
	_, err = data.GetFulfillmentBySignature(ctx, signature)
	if err == nil {
		return nil
	} else if err != fulfillment.ErrFulfillmentNotFound {
		return errors.Wrap(err, "error getting fulfillment recrd")
	}

	// Grab transaction token balances to get net Kin balances from this transaction.
	// This enables us to avoid parsing transaction data and generically handle any
	// kind of transaction. It's far too complicated if we need to inspect individual
	// instructions.
	var tokenBalances *solana.TransactionTokenBalances
	_, err = retry.Retry(
		func() error {
			// Optimizes QuickNode API credit usage
			statuses, err := data.GetBlockchainSignatureStatuses(ctx, []solana.Signature{typedSignature})
			if err != nil {
				return err
			}

			if len(statuses) == 0 || statuses[0] == nil || !statuses[0].Finalized() {
				return errSignatureNotFinalized
			}

			tokenBalances, err = data.GetBlockchainTransactionTokenBalances(ctx, signature)
			return err
		},
		waitForFinalizationRetryStrategies...,
	)
	if err != nil {
		return errors.Wrap(err, "error getting transaction token balances")
	}

	// Check whether the Code subsidizer was involved in this transaction. If it is, then
	// it cannot be an external deposit.
	for _, account := range tokenBalances.Accounts {
		if account == common.GetSubsidizer().PublicKey().ToBase58() {
			return nil
		}
	}

	var preKinBalance, postKinBalance int64
	for _, tokenBalance := range tokenBalances.PreTokenBalances {
		if tokenBalance.Mint != kin.Mint {
			continue
		}

		if tokenBalances.Accounts[tokenBalance.AccountIndex] == vault.PublicKey().ToBase58() {
			preKinBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return errors.Wrap(err, "error parsing pre token balance")
			}
			break
		}
	}
	for _, tokenBalance := range tokenBalances.PostTokenBalances {
		if tokenBalance.Mint != kin.Mint {
			continue
		}

		if tokenBalances.Accounts[tokenBalance.AccountIndex] == vault.PublicKey().ToBase58() {
			postKinBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return errors.Wrap(err, "error parsing post token balance")
			}
			break
		}
	}

	// Transaction did not positively affect toke account balance, so no new funds
	// were externally deposited into the account.
	deltaQuarks := postKinBalance - preKinBalance
	if deltaQuarks <= 0 {
		return nil
	}

	// Verified the transaction is an external deposit, so check whether we've
	// previously processed it.
	_, err = data.GetExternalDeposit(ctx, signature, vault.PublicKey().ToBase58())
	if err == nil {
		syncedDepositCache.Insert(cacheKey, true, 1)
		return nil
	}

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, vault.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}

	// Mark anything other than deposit accounts as synced and move on without
	// saving anything. There's a potential someone could overutilize our treasury
	// by depositing large sums into temporary or bucket accounts, which have
	// more lenient checks ATM. We'll deal with these adhoc as they arise.
	switch accountInfoRecord.AccountType {
	case commonpb.AccountType_PRIMARY, commonpb.AccountType_RELATIONSHIP:
	default:
		syncedDepositCache.Insert(cacheKey, true, 1)
		return nil
	}

	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
	if err != nil {
		return errors.Wrap(err, "error getting usd rate")
	}
	usdMarketValue := usdExchangeRecord.Rate * float64(deltaQuarks) / float64(kin.QuarksPerKin)

	// For a consistent payment history list
	//
	// Deprecated in favour of chats (for history purposes)
	intentRecord := &intent.Record{
		IntentId:   fmt.Sprintf("%s-%s", signature, vault.PublicKey().ToBase58()),
		IntentType: intent.ExternalDeposit,

		InitiatorOwnerAccount: tokenBalances.Accounts[0], // The fee payer

		ExternalDepositMetadata: &intent.ExternalDepositMetadata{
			DestinationOwnerAccount: accountInfoRecord.OwnerAccount,
			DestinationTokenAccount: vault.PublicKey().ToBase58(),
			Quantity:                uint64(deltaQuarks),
			UsdMarketValue:          usdMarketValue,
		},

		State:     intent.StateConfirmed,
		CreatedAt: time.Now(),
	}
	err = data.SaveIntent(ctx, intentRecord)
	if err != nil {
		return errors.Wrap(err, "error saving intent record")
	}

	err = chat_util.SendCashTransactionsExchangeMessage(ctx, data, intentRecord)
	if err != nil {
		return errors.Wrap(err, "error updating cash transactions chat")
	}

	// For tracking in balances
	externalDepositRecord := &deposit.Record{
		Signature:      signature,
		Destination:    vault.PublicKey().ToBase58(),
		Amount:         uint64(deltaQuarks),
		UsdMarketValue: usdMarketValue,

		Slot:              tokenBalances.Slot,
		ConfirmationState: transaction.ConfirmationFinalized,

		CreatedAt: time.Now(),
	}
	err = data.SaveExternalDeposit(ctx, externalDepositRecord)
	if err != nil {
		return errors.Wrap(err, "error creating external deposit record")
	}
	syncedDepositCache.Insert(cacheKey, true, 1)
	push.SendDepositPushNotification(ctx, data, pusher, vault, uint64(deltaQuarks))

	return nil
}

func markDepositsAsSynced(ctx context.Context, data code_data.Provider, vault *common.Account) error {
	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, vault.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}

	accountInfoRecord.RequiresDepositSync = false
	accountInfoRecord.DepositsLastSyncedAt = time.Now()

	err = data.UpdateAccountInfo(ctx, accountInfoRecord)
	if err != nil {
		return errors.Wrap(err, "error updating account info record")
	}
	return nil
}

func getSyncedDepositCacheKey(signature string, account *common.Account) string {
	return fmt.Sprintf("%s:%s", signature, account.PublicKey().ToBase58())
}
