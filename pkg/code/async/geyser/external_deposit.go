package async_geyser

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	indexerpb "github.com/code-payments/code-vm-indexer/generated/indexer/v1"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
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

func fixMissingExternalDeposits(ctx context.Context, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, integration Integration, userAuthority, mint *common.Account) error {
	err := maybeInitiateExternalDepositIntoVm(ctx, data, vmIndexerClient, userAuthority, mint)
	if err != nil {
		return errors.Wrap(err, "error depositing into the vm")
	}

	signatures, err := findPotentialExternalDepositsIntoVm(ctx, data, userAuthority, mint)
	if err != nil {
		return errors.Wrap(err, "error finding potential external deposits into vm")
	}

	var anyError error
	for _, signature := range signatures {
		err := processPotentialExternalDepositIntoVm(ctx, data, integration, signature, userAuthority, mint)
		if err != nil {
			anyError = errors.Wrap(err, "error processing signature for external deposit into vm")
		}
	}
	if anyError != nil {
		return anyError
	}

	return markDepositsAsSynced(ctx, data, userAuthority, mint)
}

func maybeInitiateExternalDepositIntoVm(ctx context.Context, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, userAuthority, mint *common.Account) error {
	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return err
	}

	vmDepositAccounts, err := userAuthority.GetVmDepositAccounts(vmConfig)
	if err != nil {
		return errors.Wrap(err, "error getting vm deposit ata")
	}

	balance, _, err := data.GetBlockchainBalance(ctx, vmDepositAccounts.Ata.PublicKey().ToBase58())
	if err == solana.ErrNoBalance {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "error getting vm deposit ata balance from blockchain")
	}

	if balance == 0 {
		return nil
	}
	return initiateExternalDepositIntoVm(ctx, data, vmIndexerClient, userAuthority, mint, balance)
}

func initiateExternalDepositIntoVm(ctx context.Context, data code_data.Provider, vmIndexerClient indexerpb.IndexerClient, userAuthority, mint *common.Account, balance uint64) error {
	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return errors.Wrap(err, "error getting vm config")
	}

	timelockAccounts, err := userAuthority.GetTimelockAccounts(vmConfig)
	if err != nil {
		return errors.Wrap(err, "error getting timelock accounts")
	}

	err = common.EnsureVirtualTimelockAccountIsInitialized(ctx, data, vmIndexerClient, mint, userAuthority, true)
	if err != nil {
		return errors.Wrap(err, "error ensuring vta is initialized")
	}

	memoryAccount, memoryIndex, err := common.GetVirtualTimelockAccountLocationInMemory(ctx, vmIndexerClient, vmConfig.Vm, userAuthority)
	if err != nil {
		return errors.Wrap(err, "error getting vta location in memory")
	}

	txn := solana.NewLegacyTransaction(
		vmConfig.Authority.PublicKey().ToBytes(),
		memo.Instruction(codeVmDepositMemoValue),
		compute_budget.SetComputeUnitPrice(1_000),
		compute_budget.SetComputeUnitLimit(50_000),
		cvm.NewDepositFromPdaInstruction(
			&cvm.DepositFromPdaInstructionAccounts{
				VmAuthority: vmConfig.Authority.PublicKey().ToBytes(),
				Vm:          vmConfig.Vm.PublicKey().ToBytes(),
				VmMemory:    memoryAccount.PublicKey().ToBytes(),
				Depositor:   timelockAccounts.VaultOwner.PublicKey().ToBytes(),
				DepositPda:  timelockAccounts.VmDepositAccounts.Pda.PublicKey().ToBytes(),
				DepositAta:  timelockAccounts.VmDepositAccounts.Ata.PublicKey().ToBytes(),
				VmOmnibus:   vmConfig.Omnibus.PublicKey().ToBytes(),
			},
			&cvm.DepositFromPdaInstructionArgs{
				AccountIndex: memoryIndex,
				Amount:       balance,
				Bump:         timelockAccounts.VmDepositAccounts.PdaBump,
			},
		),
	)

	bh, err := data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting latest blockhash")
	}
	txn.SetBlockhash(bh)

	err = txn.Sign(vmConfig.Authority.PrivateKey().ToBytes())
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
			confirmedTxn, err = data.GetBlockchainTransaction(ctx, base58.Encode(signature[:]), solana.CommitmentConfirmed)
			return err
		},
		waitForConfirmationRetryStrategies...,
	)
	if err != nil {
		return errors.Wrap(err, "error getting confirmed transaction")
	} else if confirmedTxn.Err != nil || confirmedTxn.Meta.Err != nil {
		return errors.New("transaction failed")
	}
	return nil
}

func findPotentialExternalDepositsIntoVm(ctx context.Context, data code_data.Provider, userAuthority, mint *common.Account) ([]string, error) {
	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return nil, err
	}

	vmDepositAta, err := userAuthority.ToVmDepositAta(vmConfig)
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

func processPotentialExternalDepositIntoVm(ctx context.Context, data code_data.Provider, integration Integration, signature string, userAuthority, mint *common.Account) error {
	vmConfig, err := common.GetVmConfigForMint(ctx, data, mint)
	if err != nil {
		return err
	}

	vmDepositAta, err := userAuthority.ToVmDepositAta(vmConfig)
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

	// Check whether the VM authority or subsidizer was involved in this transaction
	// as a payer. If not, then it couldn't have been deposited into the VM
	if tokenBalances.Accounts[0] != vmConfig.Authority.PublicKey().ToBase58() && tokenBalances.Accounts[0] != common.GetSubsidizer().PublicKey().ToBase58() {
		return nil
	}

	deltaQuarksIntoOmnibus, err := transaction_util.GetDeltaQuarksFromTokenBalances(vmConfig.Omnibus, tokenBalances)
	if err != nil {
		return errors.Wrap(err, "error getting delta quarks for vm omnibus from token balances")
	}
	deltaQuarksOutOfVmDepositAta, err := transaction_util.GetDeltaQuarksFromTokenBalances(vmDepositAta, tokenBalances)
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

	accountInfoRecordsByMint, err := data.GetAccountInfoByAuthorityAddress(ctx, userAuthority.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}
	accountInfoRecord, ok := accountInfoRecordsByMint[mint.PublicKey().ToBase58()]
	if !ok {
		return errors.New("core mint account info record doesn't exist")
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
		} else if err != deposit.ErrDepositNotFound {
			return errors.Wrap(err, "error checking for existing external deposit record")
		}

		ownerAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
		if err != nil {
			return errors.Wrap(err, "invalid owner account")
		}

		usdMarketValue, _, err := currency_util.CalculateUsdMarketValue(ctx, data, mint, uint64(deltaQuarksIntoOmnibus), time.Now())
		if err != nil {
			return errors.Wrap(err, "error calculating usd market value")
		}

		err = data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
			// For transaction history
			intentRecord := &intent.Record{
				IntentId:   getExternalDepositIntentID(signature, userVirtualTimelockVaultAccount),
				IntentType: intent.ExternalDeposit,

				MintAccount: mint.PublicKey().ToBase58(),

				InitiatorOwnerAccount: ownerAccount.PublicKey().ToBase58(),

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

			return nil
		})
		if err != nil {
			return err
		}

		syncedDepositCache.Insert(cacheKey, true, 1)

		currencyName := common.CoreMintName
		if !common.IsCoreMint(mint) {
			currencyMetadata, err := data.GetCurrencyMetadata(ctx, mint.PublicKey().ToBase58())
			if err != nil {
				return nil
			}
			currencyName = currencyMetadata.Name
		}

		// Best-effort processing for notification back to the user
		integration.OnDepositReceived(ctx, ownerAccount, mint, currencyName, usdMarketValue)

		return nil
	default:
		syncedDepositCache.Insert(cacheKey, true, 1)
		return nil
	}
}

func markDepositsAsSynced(ctx context.Context, data code_data.Provider, userAuthority, mint *common.Account) error {
	accountInfoRecords, err := data.GetAccountInfoByAuthorityAddress(ctx, userAuthority.PublicKey().ToBase58())
	if err != nil {
		return errors.Wrap(err, "error getting account info record")
	}
	accountInfoRecord, ok := accountInfoRecords[mint.PublicKey().ToBase58()]
	if !ok {
		return errors.New("core mint account info record doesn't exist")
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
