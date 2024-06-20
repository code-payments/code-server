package async_geyser

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/cache"
	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/balance"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/onramp"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
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

func fixMissingExternalDeposits(ctx context.Context, conf *conf, data code_data.Provider, pusher push_lib.Provider, vault *common.Account) error {
	signatures, err := findPotentialExternalDeposits(ctx, data, vault)
	if err != nil {
		return errors.Wrap(err, "error finding potential external deposits")
	}

	var anyError error
	for _, signature := range signatures {
		err := processPotentialExternalDeposit(ctx, conf, data, pusher, signature, vault)
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

func processPotentialExternalDeposit(ctx context.Context, conf *conf, data code_data.Provider, pusher push_lib.Provider, signature string, tokenAccount *common.Account) error {
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

	blockTime := time.Now()
	if tokenBalances.BlockTime != nil {
		blockTime = *tokenBalances.BlockTime
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

		isCodeSwap, usdcSwapAccount, usdcQuarksSwapped, err := getCodeSwapMetadata(ctx, conf, tokenBalances)
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

		if isCodeSwap {
			// Checkpoint the Code swap account balance, to minimize chances a
			// stale RPC node results in a double counting of funds
			bestEffortCacheExternalAccountBalance(ctx, data, usdcSwapAccount, tokenBalances)
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
			purchases, err := getPurchasesFromSwap(
				ctx,
				conf,
				data,
				signature,
				usdcSwapAccount,
				usdcQuarksSwapped,
			)
			if err != nil {
				return errors.Wrap(err, "error getting swap purchases")
			}

			var protoPurchases []*transactionpb.ExchangeDataWithoutRate
			for _, purchase := range purchases {
				protoPurchases = append(protoPurchases, purchase.protoExchangeData)
				recordBuyModulePurchaseCompletedEvent(
					ctx,
					purchase.deviceType,
					purchase.purchaseInitiationTime,
					purchase.usdcDepositTime,
				)
			}

			chatMessage, err := chat_util.ToKinAvailableForUseMessage(signature, blockTime, protoPurchases...)
			if err != nil {
				return errors.Wrap(err, "error creating chat message")
			}

			canPush, err := chat_util.SendKinPurchasesMessage(ctx, data, chatMessageReceiver, chatMessage)
			switch err {
			case nil:
				if canPush {
					push.SendChatMessagePushNotification(
						ctx,
						data,
						pusher,
						chat_util.KinPurchasesName,
						chatMessageReceiver,
						chatMessage,
					)
				}
			case chat_v1.ErrMessageAlreadyExists:
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
		bestEffortCacheExternalAccountBalance(ctx, data, tokenAccount, tokenBalances)

		//		go delayedUsdcDepositProcessing(
		//			ctx,
		//			conf,
		//			data,
		//			pusher,
		//			chatMessageReceiver,
		//			tokenAccount,
		//			signature,
		//			blockTime,
		//		)

		owner, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
		if err != nil {
			return errors.Wrap(err, "invalid owner account")
		}

		// Best-effort attempt to get the client to trigger a Swap RPC call now
		go push.SendTriggerSwapRpcPushNotification(
			ctx,
			data,
			pusher,
			owner,
		)

		// Have the account pulled by the swap retry worker
		err = markRequiringSwapRetries(ctx, data, accountInfoRecord)
		if err != nil {
			return err
		}

		syncedDepositCache.Insert(cacheKey, true, 1)

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

func markRequiringSwapRetries(ctx context.Context, data code_data.Provider, accountInfoRecord *account.Record) error {
	if accountInfoRecord.RequiresSwapRetry {
		return nil
	}

	accountInfoRecord.RequiresSwapRetry = true
	accountInfoRecord.LastSwapRetryAt = time.Now()
	err := data.UpdateAccountInfo(ctx, accountInfoRecord)
	if err != nil {
		return errors.Wrap(err, "error updating swap account info")
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

// todo: can be promoted more broadly
func getPostQuarkBalance(tokenAccount *common.Account, tokenBalances *solana.TransactionTokenBalances) (uint64, error) {
	for _, postBalance := range tokenBalances.PostTokenBalances {
		if tokenBalances.Accounts[postBalance.AccountIndex] == tokenAccount.PublicKey().ToBase58() {
			postQuarkBalance, err := strconv.ParseUint(postBalance.TokenAmount.Amount, 10, 64)
			if err != nil {
				return 0, errors.Wrap(err, "error parsing post token balance")
			}
			return postQuarkBalance, nil
		}
	}
	return 0, errors.New("no post balance for account")
}

func getCodeSwapMetadata(ctx context.Context, conf *conf, tokenBalances *solana.TransactionTokenBalances) (bool, *common.Account, uint64, error) {
	// Detect whether this is a Code swap by inspecting whether the swap subsidizer
	// is included in the transaction.
	var isCodeSwap bool
	for _, account := range tokenBalances.Accounts {
		if account == conf.swapSubsidizerPublicKey.Get(ctx) {
			isCodeSwap = true
			break
		}
	}

	if !isCodeSwap {
		return false, nil, 0, nil
	}

	var usdcPaid uint64
	var usdcAccount *common.Account
	for _, tokenBalance := range tokenBalances.PreTokenBalances {
		tokenAccount, err := common.NewAccountFromPublicKeyString(tokenBalances.Accounts[tokenBalance.AccountIndex])
		if err != nil {
			return false, nil, 0, errors.Wrap(err, "invalid token account")
		}

		if tokenBalance.Mint == common.UsdcMintAccount.PublicKey().ToBase58() {
			deltaQuarks, err := getDeltaQuarksFromTokenBalances(tokenAccount, tokenBalances)
			if err != nil {
				return false, nil, 0, errors.Wrap(err, "error getting delta quarks")
			}

			if deltaQuarks >= 0 {
				continue
			}

			absDeltaQuarks := uint64(-1 * deltaQuarks)
			if absDeltaQuarks > usdcPaid {
				usdcPaid = absDeltaQuarks
				usdcAccount = tokenAccount
			}
		}
	}

	if usdcAccount == nil {
		return false, nil, 0, errors.New("usdc account not found")
	}

	return true, usdcAccount, usdcPaid, nil
}

type purchaseWithMetrics struct {
	protoExchangeData      *transactionpb.ExchangeDataWithoutRate
	deviceType             client.DeviceType
	purchaseInitiationTime *time.Time
	usdcDepositTime        *time.Time
}

func getPurchasesFromSwap(
	ctx context.Context,
	conf *conf,
	data code_data.Provider,
	signature string,
	usdcSwapAccount *common.Account,
	usdcQuarksSwapped uint64,
) ([]*purchaseWithMetrics, error) {
	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, usdcSwapAccount.PublicKey().ToBase58())
	if err != nil {
		return nil, errors.Wrap(err, "error getting account info record")
	} else if accountInfoRecord.AccountType != commonpb.AccountType_SWAP {
		return nil, errors.New("usdc account is not a code swap account")
	}

	cursorValue, err := base58.Decode(signature)
	if err != nil {
		return nil, err
	}

	pageSize := 32
	history, err := data.GetBlockchainHistory(
		ctx,
		usdcSwapAccount.PublicKey().ToBase58(),
		solana.CommitmentFinalized,
		query.WithCursor(cursorValue),
		query.WithLimit(uint64(pageSize)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "error getting transaction history")
	}

	purchases, err := func() ([]*purchaseWithMetrics, error) {
		var res []*purchaseWithMetrics
		var usdcDeposited uint64
		for _, historyItem := range history {
			if historyItem.Err != nil {
				continue
			}

			tokenBalances, err := data.GetBlockchainTransactionTokenBalances(ctx, base58.Encode(historyItem.Signature[:]))
			if err != nil {
				return nil, errors.Wrap(err, "error getting token balances")
			}
			blockTime := time.Now()
			if historyItem.BlockTime != nil {
				blockTime = *historyItem.BlockTime
			}

			isCodeSwap, _, _, err := getCodeSwapMetadata(ctx, conf, tokenBalances)
			if err != nil {
				return nil, errors.Wrap(err, "error getting code swap metadata")
			}

			// Found another swap, so stop searching for purchases
			if isCodeSwap {
				// The amount of USDC deposited doesn't equate to the amount we
				// swapped. There's either a race condition between a swap and
				// deposit, or the user is manually moving funds in the account.
				//
				// Either way, the current algorithm can't properly assign pruchases
				// to the swap, so return an empty result.
				if usdcDeposited != usdcQuarksSwapped {
					return nil, nil
				}

				return res, nil
			}

			deltaQuarks, err := getDeltaQuarksFromTokenBalances(usdcSwapAccount, tokenBalances)
			if err != nil {
				return nil, errors.Wrap(err, "error getting delta usdc from token balances")
			}

			// Skip any USDC withdrawals. The average user will not be able to
			// do this anyways since the swap account is derived off the 12 words.
			if deltaQuarks <= 0 {
				continue
			}

			usdAmount := float64(deltaQuarks) / float64(usdc.QuarksPerUsdc)
			usdcDeposited += uint64(deltaQuarks)

			// Disregard any USDC deposits for inconsequential amounts to avoid
			// spam coming through
			if usdAmount < 0.01 {
				continue
			}

			rawUsdPurchase := &purchaseWithMetrics{
				protoExchangeData: &transactionpb.ExchangeDataWithoutRate{
					Currency:     "usd",
					NativeAmount: math.Round(usdAmount), // Round to nearest $1 since we don't support decimals in app yet
				},
				deviceType:      client.DeviceTypeUnknown,
				usdcDepositTime: historyItem.BlockTime,
			}

			// There is no memo for a blockchain message
			if historyItem.Memo == nil {
				res = append([]*purchaseWithMetrics{rawUsdPurchase}, res...)
				continue
			}

			// Attempt to parse a blockchain message from the memo, which will contain a
			// nonce that maps to a fiat purchase from an onramp.
			memoParts := strings.Split(*historyItem.Memo, " ")
			memoMessage := memoParts[len(memoParts)-1]
			blockchainMessage, err := thirdparty.DecodeFiatOnrampPurchaseMessage([]byte(memoMessage))
			if err != nil {
				res = append([]*purchaseWithMetrics{rawUsdPurchase}, res...)
				continue
			}
			onrampRecord, err := data.GetFiatOnrampPurchase(ctx, blockchainMessage.Nonce)
			if err == onramp.ErrPurchaseNotFound {
				res = append([]*purchaseWithMetrics{rawUsdPurchase}, res...)
				continue
			} else if err != nil {
				return nil, errors.Wrap(err, "error getting onramp record")
			}

			rawUsdPurchase.deviceType = client.DeviceType(onrampRecord.Platform)
			rawUsdPurchase.purchaseInitiationTime = &onrampRecord.CreatedAt

			// This nonce is not associated with the owner account linked to the
			// fiat purchase.
			if onrampRecord.Owner != accountInfoRecord.OwnerAccount {
				res = append([]*purchaseWithMetrics{rawUsdPurchase}, res...)
				continue
			}

			// Ensure the amounts make some sense wrt FX rates if we have them. We
			// allow a generous buffer of 10% to account for fees that might be
			// taken off of the deposited amount.
			var usdRate, otherCurrencyRate float64
			usdRateRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, blockTime)
			if err == nil {
				usdRate = usdRateRecord.Rate
			}
			otherCurrencyRateRecord, err := data.GetExchangeRate(ctx, currency_lib.Code(onrampRecord.Currency), blockTime)
			if err == nil {
				otherCurrencyRate = otherCurrencyRateRecord.Rate
			}
			if usdRate != 0 && otherCurrencyRate != 0 {
				fxRate := otherCurrencyRate / usdRate
				pctDiff := math.Abs(usdAmount*fxRate-onrampRecord.Amount) / onrampRecord.Amount
				if pctDiff > 0.1 {
					res = append([]*purchaseWithMetrics{rawUsdPurchase}, res...)
					continue
				}
			}

			res = append([]*purchaseWithMetrics{
				{
					protoExchangeData: &transactionpb.ExchangeDataWithoutRate{
						Currency:     onrampRecord.Currency,
						NativeAmount: onrampRecord.Amount,
					},
					deviceType:             client.DeviceType(onrampRecord.Platform),
					purchaseInitiationTime: &onrampRecord.CreatedAt,
					usdcDepositTime:        historyItem.BlockTime,
				},
			}, res...)
		}

		if len(history) < pageSize {
			// At the end of history, so return the result
			return res, nil
		}
		// Didn't find another swap, so we didn't find the full purchase history.
		// Return an empty result.
		//
		// todo: Continue looking back into history
		return nil, nil
	}()
	if err != nil {
		return nil, err
	}

	if len(purchases) == 0 {
		// No purchases were returned, so defer back to the USDC amount swapped
		return []*purchaseWithMetrics{
			{
				protoExchangeData: &transactionpb.ExchangeDataWithoutRate{
					Currency:     "usd",
					NativeAmount: float64(usdcQuarksSwapped) / float64(usdc.QuarksPerUsdc),
				},
			},
		}, nil
	}
	return purchases, nil
}

func delayedUsdcDepositProcessing(
	ctx context.Context,
	conf *conf,
	data code_data.Provider,
	pusher push_lib.Provider,
	ownerAccount *common.Account,
	tokenAccount *common.Account,
	signature string,
	blockTime time.Time,
) {
	// todo: configurable
	time.Sleep(2 * time.Minute)

	history, err := data.GetBlockchainHistory(ctx, tokenAccount.PublicKey().ToBase58(), solana.CommitmentFinalized, query.WithLimit(32))
	if err != nil {
		return
	}

	var foundSignature bool
	var historyToCheck []*solana.TransactionSignature
	for _, historyItem := range history {
		if base58.Encode(historyItem.Signature[:]) == signature {
			foundSignature = true
			break
		}

		if historyItem.Err == nil {
			historyToCheck = append(historyToCheck, historyItem)
		}
	}

	// The deposit is too far in the past in history, so we opt to skip processing it.
	if !foundSignature {
		return
	}

	for _, historyItem := range historyToCheck {
		tokenBalances, err := data.GetBlockchainTransactionTokenBalances(ctx, base58.Encode(historyItem.Signature[:]))
		if err != nil {
			continue
		}

		isCodeSwap, _, _, err := getCodeSwapMetadata(ctx, conf, tokenBalances)
		if err != nil {
			continue
		}

		if isCodeSwap {
			return
		}
	}

	chatMessage, err := chat_util.ToUsdcDepositedMessage(signature, blockTime)
	if err != nil {
		return
	}

	canPush, err := chat_util.SendKinPurchasesMessage(ctx, data, ownerAccount, chatMessage)
	switch err {
	case nil:
		if canPush {
			push.SendChatMessagePushNotification(
				ctx,
				data,
				pusher,
				chat_util.KinPurchasesName,
				ownerAccount,
				chatMessage,
			)
		}
	case chat_v1.ErrMessageAlreadyExists:
	default:
		return
	}
}

// Optimistically tries to cache a balance for an external account not managed
// Code. It doesn't need to be perfect and will be lazily corrected on the next
// balance fetch with a newer state returned by a RPC node.
func bestEffortCacheExternalAccountBalance(ctx context.Context, data code_data.Provider, tokenAccount *common.Account, tokenBalances *solana.TransactionTokenBalances) {
	postBalance, err := getPostQuarkBalance(tokenAccount, tokenBalances)
	if err == nil {
		checkpointRecord := &balance.Record{
			TokenAccount:   tokenAccount.PublicKey().ToBase58(),
			Quarks:         postBalance,
			SlotCheckpoint: tokenBalances.Slot,
		}
		data.SaveBalanceCheckpoint(ctx, checkpointRecord)
	}
}

func getSyncedDepositCacheKey(signature string, account *common.Account) string {
	return fmt.Sprintf("%s:%s", signature, account.PublicKey().ToBase58())
}
