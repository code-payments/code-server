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
	"github.com/code-payments/code-server/pkg/code/data/chat"
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
	"github.com/code-payments/code-server/pkg/usdc"
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

func processPotentialExternalDeposit(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, signature string, tokenAccount *common.Account) error {
	// Avoid reprocessing deposits we've recently seen and processed. Particularly,
	// the backup process will likely be triggered in frequent bursts, so this is
	// just an optimization around that.
	cacheKey := getSyncedDepositCacheKey(signature, tokenAccount)
	_, ok := syncedDepositCache.Retrieve(cacheKey)
	if ok {
		return nil
	}

	decodedSignature, err := base58.Decode(signature)
	if err != nil {
		return errors.Wrap(err, "invalid signature")
	}
	var typedSignature solana.Signature
	copy(typedSignature[:], decodedSignature)

	// Is this transaction a fulfillment? If so, it cannot be an external deposit.
	_, err = data.GetFulfillmentBySignature(ctx, signature)
	if err == nil {
		return nil
	} else if err != fulfillment.ErrFulfillmentNotFound {
		return errors.Wrap(err, "error getting fulfillment record")
	}

	// Grab transaction token balances to get net quark balances from this transaction.
	// This enables us to avoid parsing transaction data and generically handle any
	// kind of transaction. It's far too complicated if we need to inspect individual
	// instructions.
	var tokenBalances *solana.TransactionTokenBalances
	_, err = retry.Retry(
		func() error {
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

	deltaQuarks, err := getDeltaQuarksFromTokenBalances(tokenAccount, tokenBalances)
	if err != nil {
		return errors.Wrap(err, "error getting delta quarks from token balances")
	}

	// Transaction did not positively affect token account balance, so no new funds
	// were externally deposited into the account.
	if deltaQuarks <= 0 {
		return nil
	}

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, tokenAccount.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}

	chatMessageReceiver, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
	if err != nil {
		return errors.Wrap(err, "invalid owner account")
	}

	// Use the account type to determine how we'll process this external deposit
	//
	// todo: Below logic is beginning to get messy and might be in need of a
	//       refactor soon
	switch accountInfoRecord.AccountType {

	case commonpb.AccountType_PRIMARY, commonpb.AccountType_RELATIONSHIP:
		// Check whether we've previously processed this external deposit
		_, err = data.GetExternalDeposit(ctx, signature, tokenAccount.PublicKey().ToBase58())
		if err == nil {
			syncedDepositCache.Insert(cacheKey, true, 1)
			return nil
		}

		isCodeSwap, usdcQuarksSwapped, err := getCodeSwapMetadata(tokenBalances)
		if err != nil {
			return errors.Wrap(err, "error getting code swap metadata")
		}

		var usdMarketValue float64
		if isCodeSwap {
			usdMarketValue = float64(usdcQuarksSwapped) / float64(usdc.QuarksPerUsdc)
		} else {
			usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
			if err != nil {
				return errors.Wrap(err, "error getting usd rate")
			}
			usdMarketValue = usdExchangeRecord.Rate * float64(deltaQuarks) / float64(kin.QuarksPerKin)
		}

		// For a consistent payment history list
		//
		// Deprecated in favour of chats (for history purposes)
		intentRecord := &intent.Record{
			IntentId:   fmt.Sprintf("%s-%s", signature, tokenAccount.PublicKey().ToBase58()),
			IntentType: intent.ExternalDeposit,

			InitiatorOwnerAccount: tokenBalances.Accounts[0], // The fee payer

			ExternalDepositMetadata: &intent.ExternalDepositMetadata{
				DestinationOwnerAccount: accountInfoRecord.OwnerAccount,
				DestinationTokenAccount: tokenAccount.PublicKey().ToBase58(),
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

		if isCodeSwap {
			chatMessage, err := chat_util.ToKinAvailableForUseMessage(signature, usdcQuarksSwapped, time.Now())
			if err != nil {
				return errors.Wrap(err, "error creating chat message")
			}

			canPush, err := chat_util.SendCodeTeamMessage(ctx, data, chatMessageReceiver, chatMessage)
			switch err {
			case nil:
				if canPush {
					push.SendChatMessagePushNotification(
						ctx,
						data,
						pusher,
						chat_util.CodeTeamName,
						chatMessageReceiver,
						chatMessage,
					)
				}
			case chat.ErrMessageAlreadyExists:
			default:
				return errors.Wrap(err, "error sending chat message")
			}
		} else {
			err = chat_util.SendCashTransactionsExchangeMessage(ctx, data, intentRecord)
			if err != nil {
				return errors.Wrap(err, "error updating cash transactions chat")
			}
			_, err = chat_util.SendMerchantExchangeMessage(ctx, data, intentRecord, nil)
			if err != nil {
				return errors.Wrap(err, "error updating merchant chat")
			}

			push.SendDepositPushNotification(ctx, data, pusher, tokenAccount, uint64(deltaQuarks))
		}

		// For tracking in balances
		externalDepositRecord := &deposit.Record{
			Signature:      signature,
			Destination:    tokenAccount.PublicKey().ToBase58(),
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

		return nil

	case commonpb.AccountType_SWAP:
		// todo: Don't think we need to track an external deposit record. Balances
		//       cannot be tracked using cached values.

		// todo: solana client doesn't return block time
		chatMessage, err := chat_util.ToUsdcDepositedMessage(signature, uint64(deltaQuarks), time.Now())
		if err != nil {
			return errors.Wrap(err, "error creating chat message")
		}

		canPush, err := chat_util.SendCodeTeamMessage(ctx, data, chatMessageReceiver, chatMessage)
		if err == chat.ErrMessageAlreadyExists {
			syncedDepositCache.Insert(cacheKey, true, 1)
			return nil
		} else if err != nil {
			return errors.Wrap(err, "error sending chat message")
		}

		syncedDepositCache.Insert(cacheKey, true, 1)

		if canPush {
			push.SendChatMessagePushNotification(
				ctx,
				data,
				pusher,
				chat_util.CodeTeamName,
				chatMessageReceiver,
				chatMessage,
			)
		}

		return nil

	default:
		// Mark anything other than deposit or swap accounts as synced and move on without
		// saving anything. There's a potential someone could overutilize our treasury
		// by depositing large sums into temporary or bucket accounts, which have
		// more lenient checks ATM. We'll deal with these adhoc as they arise.
		syncedDepositCache.Insert(cacheKey, true, 1)
		return nil
	}
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

// todo: can be promoted more broadly
func getDeltaQuarksFromTokenBalances(tokenAccount *common.Account, tokenBalances *solana.TransactionTokenBalances) (int64, error) {
	var preQuarkBalance, postQuarkBalance int64
	var err error
	for _, tokenBalance := range tokenBalances.PreTokenBalances {
		if tokenBalances.Accounts[tokenBalance.AccountIndex] == tokenAccount.PublicKey().ToBase58() {
			preQuarkBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "error parsing pre token balance")
			}
			break
		}
	}
	for _, tokenBalance := range tokenBalances.PostTokenBalances {
		if tokenBalances.Accounts[tokenBalance.AccountIndex] == tokenAccount.PublicKey().ToBase58() {
			postQuarkBalance, err = strconv.ParseInt(tokenBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "error parsing post token balance")
			}
			break
		}
	}

	return postQuarkBalance - preQuarkBalance, nil
}

func getCodeSwapMetadata(tokenBalances *solana.TransactionTokenBalances) (bool, uint64, error) {
	// Detect whether this is a Code swap by inspecting whether the swap subsidizer
	// is included in the transaction.
	var isCodeSwap bool
	for _, account := range tokenBalances.Accounts {
		// todo: configurable
		if account == "swapBMF2EzkHSn9NDwaSFWMtGC7ZsgzApQv9NSkeUeU" {
			isCodeSwap = true
			break
		}
	}

	if !isCodeSwap {
		return false, 0, nil
	}

	var usdcPaid uint64
	for _, tokenBalance := range tokenBalances.PreTokenBalances {
		tokenAccount, err := common.NewAccountFromPublicKeyString(tokenBalances.Accounts[tokenBalance.AccountIndex])
		if err != nil {
			return false, 0, errors.Wrap(err, "invalid token account")
		}

		if tokenBalance.Mint == common.UsdcMintAccount.PublicKey().ToBase58() {
			deltaQuarks, err := getDeltaQuarksFromTokenBalances(tokenAccount, tokenBalances)
			if err != nil {
				return false, 0, errors.Wrap(err, "error getting delta quarks")
			}

			if deltaQuarks > 0 {
				continue
			}

			absDeltaQuarks := uint64(-1 * deltaQuarks)
			if absDeltaQuarks > usdcPaid {
				usdcPaid = absDeltaQuarks
			}
		}
	}

	return true, usdcPaid, nil
}

func getSyncedDepositCacheKey(signature string, account *common.Account) string {
	return fmt.Sprintf("%s:%s", signature, account.PublicKey().ToBase58())
}
