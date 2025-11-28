package async_swap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/swap"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/memo"
	"github.com/code-payments/code-server/pkg/solana/system"
)

func (p *service) validateSwapState(record *swap.Record, states ...swap.State) error {
	if slices.Contains(states, record.State) {
		return nil
	}
	return errors.New("invalid swap state")
}

func (p *service) markSwapFunded(ctx context.Context, record *swap.Record) error {
	err := p.validateSwapState(record, swap.StateFunding)
	if err != nil {
		return err
	}

	record.State = swap.StateFunded
	return p.data.SaveSwap(ctx, record)
}

func (p *service) markSwapFinalized(ctx context.Context, record *swap.Record) error {
	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := p.validateSwapState(record, swap.StateSubmitting)
		if err != nil {
			return err
		}

		err = p.markNonceReleasedDueToSubmittedTransaction(ctx, record)
		if err != nil {
			return err
		}

		record.TransactionBlob = nil
		record.State = swap.StateFinalized
		return p.data.SaveSwap(ctx, record)
	})
}

func (p *service) markSwapFailed(ctx context.Context, record *swap.Record) error {
	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := p.validateSwapState(record, swap.StateSubmitting)
		if err != nil {
			return err
		}

		err = p.markNonceReleasedDueToSubmittedTransaction(ctx, record)
		if err != nil {
			return err
		}

		record.TransactionBlob = nil
		record.State = swap.StateFailed
		return p.data.SaveSwap(ctx, record)
	})
}

func (p *service) markSwapCancelling(ctx context.Context, record *swap.Record, txn *solana.Transaction) error {
	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := p.validateSwapState(record, swap.StateFunded)
		if err != nil {
			return err
		}

		txnSignature := base58.Encode(txn.Signature())

		err = transaction_util.UpdateNonceSignature(ctx, p.data, record.Nonce, record.ProofSignature, txnSignature)
		if err != nil {
			return err
		}

		record.TransactionSignature = &txnSignature
		record.TransactionBlob = txn.Marshal()
		record.State = swap.StateCancelling
		return p.data.SaveSwap(ctx, record)
	})
}

func (p *service) markSwapCancelled(ctx context.Context, record *swap.Record) error {
	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := p.validateSwapState(record, swap.StateCreated, swap.StateCancelling)
		if err != nil {
			return err
		}

		switch record.State {
		case swap.StateCreated:
			err = p.markNonceAvailableDueToCancelledSwap(ctx, record)
			if err != nil {
				return err
			}
		case swap.StateCancelling:
			err = p.markNonceReleasedDueToSubmittedTransaction(ctx, record)
			if err != nil {
				return err
			}
		}

		record.TransactionBlob = nil
		record.State = swap.StateCancelled
		return p.data.SaveSwap(ctx, record)
	})
}

func (p *service) submitTransaction(ctx context.Context, record *swap.Record) error {
	err := p.validateSwapState(record, swap.StateSubmitting, swap.StateCancelling)
	if err != nil {
		return err
	}

	var txn solana.Transaction
	err = txn.Unmarshal(record.TransactionBlob)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling transaction")
	}

	if base58.Encode(txn.Signature()) != *record.TransactionSignature {
		return errors.New("unexpected transaction signature")
	}

	_, err = p.data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		return errors.Wrap(err, "error submitting transaction")
	}
	return nil
}

func (p *service) updateBalancesForFinalizedSwap(ctx context.Context, record *swap.Record) (uint64, error) {
	owner, err := common.NewAccountFromPublicKeyString(record.Owner)
	if err != nil {
		return 0, err
	}

	toMint, err := common.NewAccountFromPublicKeyString(record.ToMint)
	if err != nil {
		return 0, err
	}

	destinationVmConfig, err := common.GetVmConfigForMint(ctx, p.data, toMint)
	if err != nil {
		return 0, err
	}

	ownerDestinationTimelockVault, err := owner.ToTimelockVault(destinationVmConfig)
	if err != nil {
		return 0, err
	}

	tokenBalances, err := p.data.GetBlockchainTransactionTokenBalances(ctx, *record.TransactionSignature)
	if err != nil {
		return 0, err
	}

	deltaQuarksIntoOmnibus, err := transaction_util.GetDeltaQuarksFromTokenBalances(destinationVmConfig.Omnibus, tokenBalances)
	if err != nil {
		return 0, err
	}
	if deltaQuarksIntoOmnibus <= 0 {
		return 0, errors.New("delta quarks into destination vm omnibus is not positive")
	}

	usdMarketValue, _, err := currency_util.CalculateUsdMarketValue(ctx, p.data, toMint, uint64(deltaQuarksIntoOmnibus), time.Now())
	if err != nil {
		return 0, err
	}

	err = p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		// For transaction history
		intentRecord := &intent.Record{
			IntentId:   getSwapDepositIntentID(*record.TransactionSignature, ownerDestinationTimelockVault),
			IntentType: intent.ExternalDeposit,

			MintAccount: toMint.PublicKey().ToBase58(),

			InitiatorOwnerAccount: owner.PublicKey().ToBase58(),

			ExternalDepositMetadata: &intent.ExternalDepositMetadata{
				DestinationTokenAccount: ownerDestinationTimelockVault.PublicKey().ToBase58(),
				Quantity:                uint64(deltaQuarksIntoOmnibus),
				UsdMarketValue:          usdMarketValue,
			},

			State:     intent.StateConfirmed,
			CreatedAt: time.Now(),
		}
		err = p.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			return err
		}

		// For tracking in cached balances
		externalDepositRecord := &deposit.Record{
			Signature:      *record.TransactionSignature,
			Destination:    ownerDestinationTimelockVault.PublicKey().ToBase58(),
			Amount:         uint64(deltaQuarksIntoOmnibus),
			UsdMarketValue: usdMarketValue,

			Slot:              tokenBalances.Slot,
			ConfirmationState: transaction.ConfirmationFinalized,

			CreatedAt: time.Now(),
		}
		return p.data.SaveExternalDeposit(ctx, externalDepositRecord)
	})
	if err != nil {
		return 0, err
	}
	return uint64(deltaQuarksIntoOmnibus), nil
}

func (p *service) updateBalancesForCancelledSwap(ctx context.Context, record *swap.Record) error {
	owner, err := common.NewAccountFromPublicKeyString(record.Owner)
	if err != nil {
		return err
	}

	fromMint, err := common.NewAccountFromPublicKeyString(record.FromMint)
	if err != nil {
		return err
	}

	soureVmConfig, err := common.GetVmConfigForMint(ctx, p.data, fromMint)
	if err != nil {
		return err
	}

	ownerSourceTimelockVault, err := owner.ToTimelockVault(soureVmConfig)
	if err != nil {
		return err
	}

	tokenBalances, err := p.data.GetBlockchainTransactionTokenBalances(ctx, *record.TransactionSignature)
	if err != nil {
		return err
	}

	deltaQuarksIntoOmnibus, err := transaction_util.GetDeltaQuarksFromTokenBalances(soureVmConfig.Omnibus, tokenBalances)
	if err != nil {
		return err
	}
	if deltaQuarksIntoOmnibus <= 0 {
		return errors.New("delta quarks into destination vm omnibus is not positive")
	}

	usdMarketValue, _, err := currency_util.CalculateUsdMarketValue(ctx, p.data, fromMint, uint64(deltaQuarksIntoOmnibus), time.Now())
	if err != nil {
		return err
	}

	return p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		// For transaction history
		intentRecord := &intent.Record{
			IntentId:   getSwapDepositIntentID(*record.TransactionSignature, ownerSourceTimelockVault),
			IntentType: intent.ExternalDeposit,

			MintAccount: fromMint.PublicKey().ToBase58(),

			InitiatorOwnerAccount: owner.PublicKey().ToBase58(),

			ExternalDepositMetadata: &intent.ExternalDepositMetadata{
				DestinationTokenAccount: ownerSourceTimelockVault.PublicKey().ToBase58(),
				Quantity:                uint64(deltaQuarksIntoOmnibus),
				UsdMarketValue:          usdMarketValue,
			},

			State:     intent.StateConfirmed,
			CreatedAt: time.Now(),
		}
		err = p.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			return err
		}

		// For tracking in cached balances
		externalDepositRecord := &deposit.Record{
			Signature:      *record.TransactionSignature,
			Destination:    ownerSourceTimelockVault.PublicKey().ToBase58(),
			Amount:         uint64(deltaQuarksIntoOmnibus),
			UsdMarketValue: usdMarketValue,

			Slot:              tokenBalances.Slot,
			ConfirmationState: transaction.ConfirmationFinalized,

			CreatedAt: time.Now(),
		}
		return p.data.SaveExternalDeposit(ctx, externalDepositRecord)
	})
}

func (p *service) notifySwapFinalized(ctx context.Context, swapRecord *swap.Record) error {
	owner, err := common.NewAccountFromPublicKeyString(swapRecord.Owner)
	if err != nil {
		return err
	}

	toMint, err := common.NewAccountFromPublicKeyString(swapRecord.ToMint)
	if err != nil {
		return err
	}

	currencyName := common.CoreMintName
	if !common.IsCoreMint(toMint) {
		currencyMetadataRecord, err := p.data.GetCurrencyMetadata(ctx, toMint.PublicKey().ToBase58())
		if err != nil {
			return nil
		}
		currencyName = currencyMetadataRecord.Name
	}

	fundingIntentRecord, err := p.data.GetIntent(ctx, swapRecord.FundingId)
	if err != nil {
		return err
	}

	return p.integration.OnSwapFinalized(ctx, owner, toMint, currencyName, fundingIntentRecord.SendPublicPaymentMetadata.ExchangeCurrency, fundingIntentRecord.SendPublicPaymentMetadata.NativeAmount)
}

// todo: put this in transaction utility package
func (p *service) getCancellationTransaction(ctx context.Context, record *swap.Record) (*solana.Transaction, error) {
	owner, err := common.NewAccountFromPublicKeyString(record.Owner)
	if err != nil {
		return nil, err
	}

	fromMint, err := common.NewAccountFromPublicKeyString(record.FromMint)
	if err != nil {
		return nil, err
	}

	nonce, err := common.NewAccountFromPublicKeyString(record.Nonce)
	if err != nil {
		return nil, err
	}

	decodedBlockhash, err := base58.Decode(record.Blockhash)
	if err != nil {
		return nil, err
	}

	sourceVmConfig, err := common.GetVmConfigForMint(ctx, p.data, fromMint)
	if err != nil {
		return nil, err
	}

	sourceOwnerVmSwapPdaAccounts, err := owner.GetVmSwapAccounts(sourceVmConfig)
	if err != nil {
		return nil, err
	}

	memoryAccount, memoryIndex, err := common.GetVirtualTimelockAccountLocationInMemory(ctx, p.vmIndexerClient, sourceVmConfig.Vm, owner)
	if err != nil {
		return nil, err
	}

	txn := solana.NewLegacyTransaction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		system.AdvanceNonce(nonce.PublicKey().ToBytes(), common.GetSubsidizer().PublicKey().ToBytes()),
		compute_budget.SetComputeUnitLimit(200_000), // todo: optimize this
		compute_budget.SetComputeUnitPrice(1_000),
		memo.Instruction("cancel_swap_v0"),
		cvm.NewCancelSwapInstruction(
			&cvm.CancelSwapInstructionAccounts{
				VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
				Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
				VmMemory:    memoryAccount.PublicKey().ToBytes(),
				Swapper:     owner.PublicKey().ToBytes(),
				SwapPda:     sourceOwnerVmSwapPdaAccounts.Pda.PublicKey().ToBytes(),
				SwapAta:     sourceOwnerVmSwapPdaAccounts.Ata.PublicKey().ToBytes(),
				VmOmnibus:   sourceVmConfig.Omnibus.PublicKey().ToBytes(),
			},
			&cvm.CancelSwapInstructionArgs{
				AccountIndex: memoryIndex,
				Amount:       record.Amount,
				Bump:         sourceOwnerVmSwapPdaAccounts.PdaBump,
			},
		),
		cvm.NewCloseSwapAccountIfEmptyInstruction(
			&cvm.CloseSwapAccountIfEmptyInstructionAccounts{
				VmAuthority: sourceVmConfig.Authority.PublicKey().ToBytes(),
				Vm:          sourceVmConfig.Vm.PublicKey().ToBytes(),
				Swapper:     owner.PublicKey().ToBytes(),
				SwapPda:     sourceOwnerVmSwapPdaAccounts.Pda.PublicKey().ToBytes(),
				SwapAta:     sourceOwnerVmSwapPdaAccounts.Ata.PublicKey().ToBytes(),
				Destination: common.GetSubsidizer().PublicKey().ToBytes(),
			},
			&cvm.CloseSwapAccountIfEmptyInstructionArgs{
				Bump: sourceOwnerVmSwapPdaAccounts.PdaBump,
			},
		),
	)

	txn.SetBlockhash(solana.Blockhash(decodedBlockhash))

	err = txn.Sign(
		common.GetSubsidizer().PrivateKey().ToBytes(),
		sourceVmConfig.Authority.PrivateKey().ToBytes(),
	)
	if err != nil {
		return nil, err
	}

	return &txn, nil
}

func (p *service) markNonceReleasedDueToSubmittedTransaction(ctx context.Context, record *swap.Record) error {
	err := p.validateSwapState(record, swap.StateSubmitting, swap.StateCancelling)
	if err != nil {
		return err
	}

	nonceRecord, err := p.data.GetNonce(ctx, record.Nonce)
	if err != nil {
		return err
	}

	if *record.TransactionSignature != nonceRecord.Signature {
		return errors.New("unexpected nonce signature")
	}

	if record.Blockhash != nonceRecord.Blockhash {
		return errors.New("unexpected nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected nonce state")
	}

	nonceRecord.State = nonce.StateReleased
	return p.data.SaveNonce(ctx, nonceRecord)
}

func (p *service) markNonceAvailableDueToCancelledSwap(ctx context.Context, record *swap.Record) error {
	err := p.validateSwapState(record, swap.StateCreated)
	if err != nil {
		return err
	}

	nonceRecord, err := p.data.GetNonce(ctx, record.Nonce)
	if err != nil {
		return err
	}

	if record.ProofSignature != nonceRecord.Signature {
		return errors.New("unexpected nonce signature")
	}

	if record.Blockhash != nonceRecord.Blockhash {
		return errors.New("unexpected nonce blockhash")
	}

	if nonceRecord.State != nonce.StateReserved {
		return errors.New("unexpected nonce state")
	}

	nonceRecord.State = nonce.StateAvailable
	nonceRecord.Signature = ""
	return p.data.SaveNonce(ctx, nonceRecord)
}

func getSwapDepositIntentID(signature string, destination *common.Account) string {
	combined := fmt.Sprintf("%s-%s", signature, destination.PublicKey().ToBase58())
	hashed := sha256.Sum256([]byte(combined))
	return base58.Encode(hashed[:])
}
