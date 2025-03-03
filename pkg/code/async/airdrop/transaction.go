package async_airdrop

import (
	"context"
	"crypto/ed25519"
	"math"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	transaction_util "github.com/code-payments/code-server/pkg/code/transaction"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

const (
	maxAirdropsInVixn = 50
	maxAirdropsInTxn  = 200

	cuLimitPerVixn = 200_000

	txnWatchTimeout = 5 * time.Minute
)

func (p *service) airdropToOwners(ctx context.Context, amount uint64, owners ...*common.Account) error {
	type destinationInMemory struct {
		owner  *common.Account
		memory *common.Account
		index  uint16
	}

	destinationsByMemory := make(map[string][]*destinationInMemory)
	for _, owner := range owners {
		memory, index, err := p.getVirtualTimelockAccountMemoryLocation(ctx, common.CodeVmAccount, owner)
		if err == errNotIndexed {
			continue
		} else if err != nil {
			return err
		}

		destinationsByMemory[memory.PublicKey().ToBase58()] = append(destinationsByMemory[memory.PublicKey().ToBase58()], &destinationInMemory{
			owner:  owner,
			memory: memory,
			index:  index,
		})
	}

	for timelockMemoryAccount := range destinationsByMemory {
		destinationsInMemory := destinationsByMemory[timelockMemoryAccount]

		var batchDestinations []*common.Account
		var batchDestinationMemoryIndices []uint16
		for i, destinationInMemory := range destinationsInMemory {
			batchDestinations = append(batchDestinations, destinationInMemory.owner)
			batchDestinationMemoryIndices = append(batchDestinationMemoryIndices, destinationInMemory.index)

			if len(batchDestinations) >= maxAirdropsInTxn || i == len(destinationsInMemory)-1 {
				sig, err := p.submitAirdrop(ctx, amount, batchDestinations, destinationsInMemory[0].memory, batchDestinationMemoryIndices)
				if err != nil {
					return err
				}

				txn, err := p.watchTxn(ctx, sig)
				if err != nil {
					return err
				}

				err = p.onSuccess(ctx, txn, amount, batchDestinations...)
				if err != nil {
					return err
				}

				batchDestinations = nil
				batchDestinationMemoryIndices = nil
			}
		}
	}

	return nil
}

func (p *service) submitAirdrop(ctx context.Context, amount uint64, destinations []*common.Account, destinationMemoryAccount *common.Account, destinationMemoryIndices []uint16) (string, error) {
	if len(destinations) == 0 {
		return "", errors.New("no destinations")
	}
	if len(destinations) > maxAirdropsInTxn {
		return "", errors.New("too many destinations")
	}

	ixnsNeeded := int(math.Ceil(float64(len(destinations)) / float64(maxAirdropsInVixn)))
	cuLimit := uint32(cuLimitPerVixn * ixnsNeeded)

	ixns := []solana.Instruction{
		compute_budget.SetComputeUnitPrice(1_000), // todo: dynamic
		compute_budget.SetComputeUnitLimit(cuLimit),
	}

	var batchPublicKeys []ed25519.PublicKey
	var batchMemoryIndices []uint16
	batchMemoryAccounts := []*common.Account{p.nonceMemoryAccount, p.airdropperMemoryAccount}
	for i, destination := range destinations {
		batchPublicKeys = append(batchPublicKeys, destination.PublicKey().ToBytes())
		batchMemoryIndices = append(batchMemoryIndices, destinationMemoryIndices[i])
		batchMemoryAccounts = append(batchMemoryAccounts, destinationMemoryAccount)

		if len(batchPublicKeys) >= maxAirdropsInVixn || i == len(destinations)-1 {
			vdn, nonceIndex, err := p.getVdn()
			if err != nil {
				return "", err
			}

			msg := cvm.GetCompactAirdropMessage(&cvm.GetCompactAirdropMessageArgs{
				Source:       p.airdropperTimelockAccounts.Vault.PublicKey().ToBytes(),
				Destinations: batchPublicKeys,
				Amount:       amount,
				NonceAddress: vdn.Address,
				NonceValue:   vdn.Value,
			})

			vsig, err := p.airdropper.Sign(msg[:])
			if err != nil {
				return "", err
			}

			vixn := cvm.NewAirdropVirtualInstruction(
				&cvm.AirdropVirtualInstructionArgs{
					Amount:    amount,
					Count:     uint8(len(batchPublicKeys)),
					Signature: cvm.Signature(vsig),
				},
			)

			mergedMemoryBanks, err := transaction_util.MergeMemoryBanks(batchMemoryAccounts...)
			if err != nil {
				return "", err
			}

			ixns = append(ixns, cvm.NewExecInstruction(
				&cvm.ExecInstructionAccounts{
					VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
					Vm:          common.CodeVmAccount.PublicKey().ToBytes(),
					VmMemA:      mergedMemoryBanks.A,
					VmMemB:      mergedMemoryBanks.B,
					VmMemC:      mergedMemoryBanks.C,
				},
				&cvm.ExecInstructionArgs{
					Opcode:     vixn.Opcode,
					MemIndices: append([]uint16{nonceIndex, p.airdropperMemoryAccountIndex}, batchMemoryIndices...),
					MemBanks:   mergedMemoryBanks.Indices,
					Data:       vixn.Data,
				},
			))

			batchPublicKeys = nil
			batchMemoryIndices = nil
			batchMemoryAccounts = []*common.Account{p.nonceMemoryAccount, p.airdropperMemoryAccount}
		}
	}

	txn := solana.NewTransaction(common.GetSubsidizer().PublicKey().ToBytes(), ixns...)

	bh, err := p.data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		return "", err
	}
	txn.SetBlockhash(bh)

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return "", err
	}

	sig, err := p.data.SubmitBlockchainTransaction(ctx, &txn)
	if err != nil {
		return "", err
	}

	return base58.Encode(sig[:]), nil
}

func (p *service) watchTxn(ctx context.Context, sig string) (*solana.ConfirmedTransaction, error) {
	for range txnWatchTimeout / time.Second {
		time.Sleep(time.Second)

		txn, err := p.data.GetBlockchainTransaction(ctx, sig, solana.CommitmentFinalized)
		if err != nil {
			continue
		}

		if txn.Err != nil || txn.Meta.Err != nil {
			return nil, errors.New("transaction failed")
		}

		return txn, nil
	}

	return nil, errors.New("transaction didn't finalize")
}

func (p *service) onSuccess(ctx context.Context, txn *solana.ConfirmedTransaction, amount uint64, owners ...*common.Account) error {
	var usdMarketValue float64
	usdExchangeRateRecord, err := p.data.GetExchangeRate(ctx, currency.USD, *txn.BlockTime)
	if err == nil {
		usdMarketValue = usdExchangeRateRecord.Rate * float64(amount)
	} else {
		// todo: warn and move on
	}

	var vaults []*common.Account
	for _, owner := range owners {
		timelockAccounts, err := owner.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
		if err != nil {
			return err
		}
		vaults = append(vaults, timelockAccounts.Vault)
	}

	for _, vault := range vaults {
		externalDepositRecord := &deposit.Record{
			Signature:      base58.Encode(txn.Transaction.Signature()),
			Destination:    vault.PublicKey().ToBase58(),
			Amount:         amount,
			UsdMarketValue: usdMarketValue,

			Slot:              txn.Slot,
			ConfirmationState: transaction.ConfirmationFinalized,

			CreatedAt: time.Now(),
		}

		err := p.data.SaveExternalDeposit(ctx, externalDepositRecord)
		if err != nil {
			return err
		}
	}

	return p.integration.OnSuccess(ctx, owners...)
}
