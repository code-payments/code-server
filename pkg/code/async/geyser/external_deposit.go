package async_geyser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/memo"
)

const (
	// todo: something better?
	codeVmDepositMemoValue = "vm_deposit"
)

var (
	syncedDepositCache = cache.NewCache(1_000_000)
)

func fixMissingExternalDeposits(ctx context.Context, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, userAuthority *common.Account) error {
	err := maybeInitiateExternalDepositIntoVm(ctx, data, vmIndexerClient, userAuthority)
	if err != nil {
		return errors.Wrap(err, "error depositing into the vm")
	}

	signatures, err := findPotentialExternalDepositsIntoVm(ctx, data, userAuthority)
	if err != nil {
		return errors.Wrap(err, "error finding potential external deposits into vm")
	}

	var anyError error
	for _, signature := range signatures {
		err := processPotentialExternalDepositIntoVm(ctx, data, signature, userAuthority)
		if err != nil {
			anyError = errors.Wrap(err, "error processing signature for external deposit into vm")
		}
	}
	if anyError != nil {
		return anyError
	}

	return markDepositsAsSynced(ctx, data, userAuthority)
}

func maybeInitiateExternalDepositIntoVm(ctx context.Context, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, userAuthority *common.Account) error {
	vmDepositAccounts, err := userAuthority.GetVmDepositAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return errors.Wrap(err, "error getting vm deposit ata")
	}

	balance, _, err := data.GetBlockchainBalance(ctx, vmDepositAccounts.Ata.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting vm deposit ata balance from blockchain")
	}

	if balance == 0 {
		return nil
	}

	memoryAccount, memoryIndex, err := getVirtualTimelockAccountLocationInMemory(ctx, vmIndexerClient, common.CodeVmAccount, userAuthority)
	if err != nil {
		return errors.Wrap(err, "error getting vta location in memory")
	}

	txn := solana.NewTransaction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		memo.Instruction(codeVmDepositMemoValue),
		compute_budget.SetComputeUnitPrice(1_000),
		compute_budget.SetComputeUnitLimit(50_000),
		cvm.NewDepositInstruction(
			&cvm.DepositInstructionAccounts{
				VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
				Vm:          common.CodeVmAccount.PublicKey().ToBytes(),
				VmMemory:    memoryAccount.PublicKey().ToBytes(),
				Depositor:   vmDepositAccounts.VaultOwner.PublicKey().ToBytes(),
				DepositPda:  vmDepositAccounts.Pda.PublicKey().ToBytes(),
				DepositAta:  vmDepositAccounts.Ata.PublicKey().ToBytes(),
				VmOmnibus:   common.CodeVmOmnibusAccount.PublicKey().ToBytes(),
			},
			&cvm.DepositInstructionArgs{
				AccountIndex: memoryIndex,
				Amount:       balance,
				Bump:         vmDepositAccounts.PdaBump,
			},
		),
	)

	bh, err := data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting latest blockhash")
	}
	txn.SetBlockhash(bh)

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return errors.Wrap(err, "error signing transaction")
	}

	signature, err := data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		return errors.Wrap(err, "error submitting transaction to the blockchain")
	}

	var confirmedTxn *solana.ConfirmedTransaction
	_, err = retry.Retry(
		func() error {
			confirmedTxn, err = data.GetBlockchainTransaction(ctx, base58.Encode(signature[:]), solana.CommitmentFinalized)
			return err
		},
		waitForFinalizationRetryStrategies...,
	)
	if err != nil {
		return errors.Wrap(err, "error getting confirmed transaction")
	} else if confirmedTxn.Err != nil || confirmedTxn.Meta.Err != nil {
		return errors.New("transaction failed")
	}
	return nil
}

func findPotentialExternalDepositsIntoVm(ctx context.Context, data code_data.Provider, userAuthority *common.Account) ([]string, error) {
	vmDepositAta, err := userAuthority.ToVmDepositAssociatedTokenAccount(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return nil, errors.Wrap(err, "error getting vm deposit ata")
	}

	var res []string
	var cursor []byte
	var totalTransactionsFound int
	for {
		history, err := data.GetBlockchainHistory(
			ctx,
			vmDepositAta.PublicKey().ToBase58(),
			solana.CommitmentConfirmed, // Get signatures faster, which is ok because we'll fetch finalized txn data
			query.WithLimit(100),
			query.WithCursor(cursor),
		)
		if err != nil {
			return nil, errors.Wrap(err, "error getting signatures for address")
		}

		if len(history) == 0 {
			return res, nil
		}

		for _, historyItem := range history {
			// Exclude transactions that don't include the VM deposit memo
			if historyItem.Memo == nil || !strings.Contains(*historyItem.Memo, codeVmDepositMemoValue) {
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
		if totalTransactionsFound >= 1_000 {
			return res, nil
		}

		cursor = query.Cursor(history[len(history)-1].Signature[:])
	}
}

func processPotentialExternalDepositIntoVm(ctx context.Context, data code_data.Provider, signature string, userAuthority *common.Account) error {
	vmDepositAta, err := userAuthority.ToVmDepositAssociatedTokenAccount(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return errors.Wrap(err, "error getting vm deposit ata")
	}

	// Avoid reprocessing deposits we've recently seen and processed. Particularly,
	// the backup process will likely be triggered in frequent bursts, so this is
	// just an optimization around that.
	cacheKey := getSyncedVmDepositCacheKey(signature, vmDepositAta)
	_, ok := syncedDepositCache.Retrieve(cacheKey)
	if ok {
		return nil
	}

	// Grab transaction token balances to get net quark balances from this transaction.
	// This enables us to avoid parsing transaction data and generically handle any
	// kind of transaction.
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

	// Check whether the VM authority was involved in this transaction as a signer.
	// If not, then it couldn't have been deposited into the VM
	if tokenBalances.Accounts[0] != common.GetSubsidizer().PublicKey().ToBase58() {
		return nil
	}

	deltaQuarksIntoOmnibus, err := getDeltaQuarksFromTokenBalances(common.CodeVmOmnibusAccount, tokenBalances)
	if err != nil {
		return errors.Wrap(err, "error getting delta quarks for vm omnibus from token balances")
	}
	deltaQuarksOutOfVmDepositAta, err := getDeltaQuarksFromTokenBalances(vmDepositAta, tokenBalances)
	if err != nil {
		return errors.Wrap(err, "error getting delta quarks for vm deposit ata from token balances")
	}

	// Transaction did not positively affect token account balance into the VM omnibus,
	// so no new funds were externally deposited into the virtual timelock account.
	if deltaQuarksIntoOmnibus <= 0 {
		return nil
	}
	// Transaction wasn't funded by the specified VM deposit ATA
	if deltaQuarksOutOfVmDepositAta != -1*deltaQuarksIntoOmnibus {
		return nil
	}

	accountInfoRecord, err := data.GetAccountInfoByAuthorityAddress(ctx, userAuthority.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}
	userVirtualTimelockVaultAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
	if err != nil {
		return errors.Wrap(err, "invalid virtual timelock vault account")
	}

	// Use the account type to determine how we'll process this external deposit
	switch accountInfoRecord.AccountType {
	case commonpb.AccountType_PRIMARY:
		// Check whether we've previously processed this external deposit
		_, err = data.GetExternalDeposit(ctx, signature, userVirtualTimelockVaultAccount.PublicKey().ToBase58())
		if err == nil {
			syncedDepositCache.Insert(cacheKey, true, 1)
			return nil
		}

		usdExchangeRecord, err := data.GetExchangeRate(ctx, currency_lib.USD, time.Now())
		if err != nil {
			return errors.Wrap(err, "error getting usd rate")
		}
		usdMarketValue := usdExchangeRecord.Rate * float64(deltaQuarksIntoOmnibus) / float64(common.CoreMintQuarksPerUnit)

		// For transaction history
		intentRecord := &intent.Record{
			IntentId:   getExternalDepositIntentID(signature, userVirtualTimelockVaultAccount),
			IntentType: intent.ExternalDeposit,

			InitiatorOwnerAccount: accountInfoRecord.OwnerAccount,

			ExternalDepositMetadata: &intent.ExternalDepositMetadata{
				DestinationTokenAccount: userVirtualTimelockVaultAccount.PublicKey().ToBase58(),
				Quantity:                uint64(deltaQuarksIntoOmnibus),
				UsdMarketValue:          usdMarketValue,
			},

			State:     intent.StateConfirmed,
			CreatedAt: time.Now(),
		}
		err = data.SaveIntent(ctx, intentRecord)
		if err != nil {
			return errors.Wrap(err, "error saving intent record")
		}

		// For tracking in cached balances
		externalDepositRecord := &deposit.Record{
			Signature:      signature,
			Destination:    userVirtualTimelockVaultAccount.PublicKey().ToBase58(),
			Amount:         uint64(deltaQuarksIntoOmnibus),
			UsdMarketValue: usdMarketValue,

			Slot:              tokenBalances.Slot,
			ConfirmationState: transaction.ConfirmationFinalized,

			CreatedAt: time.Now(),
		}
		err = data.SaveExternalDeposit(ctx, externalDepositRecord)
		if err != nil {
			return errors.Wrap(err, "error saving external deposit record")
		}

		syncedDepositCache.Insert(cacheKey, true, 1)

		return nil
	default:
		syncedDepositCache.Insert(cacheKey, true, 1)
		return nil
	}
}

func getVirtualTimelockAccountLocationInMemory(ctx context.Context, vmIndexerClient indexerpb.IndexerClient, vm, owner *common.Account) (*common.Account, uint16, error) {
	resp, err := vmIndexerClient.GetVirtualTimelockAccounts(ctx, &indexerpb.GetVirtualTimelockAccountsRequest{
		VmAccount: &indexerpb.Address{Value: vm.PublicKey().ToBytes()},
		Owner:     &indexerpb.Address{Value: owner.PublicKey().ToBytes()},
	})
	if err != nil {
		return nil, 0, err
	} else if resp.Result != indexerpb.GetVirtualTimelockAccountsResponse_OK {
		return nil, 0, errors.Errorf("received rpc result %s", resp.Result.String())
	}

	if len(resp.Items) > 1 {
		return nil, 0, errors.New("multiple results returned")
	} else if resp.Items[0].Storage.GetMemory() == nil {
		return nil, 0, errors.New("account is compressed or hasn't been initialized")
	}

	protoMemory := resp.Items[0].Storage.GetMemory()
	memory, err := common.NewAccountFromPublicKeyBytes(protoMemory.Account.Value)
	if err != nil {
		return nil, 0, err
	}
	return memory, uint16(protoMemory.Index), nil
}

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

func markDepositsAsSynced(ctx context.Context, data code_data.Provider, userAuthority *common.Account) error {
	accountInfoRecord, err := data.GetAccountInfoByAuthorityAddress(ctx, userAuthority.PublicKey().ToBase58())
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

// Consistent intent ID that maps to a 32 byte buffer
func getExternalDepositIntentID(signature string, destination *common.Account) string {
	combined := fmt.Sprintf("%s-%s", signature, destination.PublicKey().ToBase58())
	hashed := sha256.Sum256([]byte(combined))
	return base58.Encode(hashed[:])
}

func getSyncedVmDepositCacheKey(signature string, vmDepositAta *common.Account) string {
	return fmt.Sprintf("%s:%s", signature, vmDepositAta.PublicKey().ToBase58())
}

// todo: below code needs to be reimagined for the VM

/*
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
*/
