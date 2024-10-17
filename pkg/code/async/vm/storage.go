package async_vm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/cvm/storage"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

const (
	// todo: make these configurable
	maxStorageAccountLevels   = 20     // maximum for decompression under a single tx
	minStorageAccountCapacity = 50_000 // mimumum capacity for a storage until we decide we need a new one
)

func (p *service) storageInitWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "storageInitWorker")

	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__vm_service__handle_init_storage_account")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			err = p.maybeInitStorageAccount(tracedCtx)
			if err != nil {
				m.NoticeError(err)
				log.WithError(err).Warn("failure handling init storage account")
			}
			return err
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) maybeInitStorageAccount(ctx context.Context) error {
	// todo: iterate over purposes when we have more than one
	purpose := storage.PurposeDeletion

	_, err := p.data.FindAnyVmStorageWithAvailableCapacity(ctx, common.CodeVmAccount.PublicKey().ToBase58(), purpose, minStorageAccountCapacity)
	switch err {
	case storage.ErrNotFound:
	case nil:
		return nil
	default:
		return err
	}

	record, err := p.initStorageAccountOnBlockchain(ctx, common.CodeVmAccount, purpose)
	if err != nil {
		return err
	}

	return p.data.InitializeVmStorage(ctx, record)
}

func (p *service) initStorageAccountOnBlockchain(ctx context.Context, vm *common.Account, purpose storage.Purpose) (*storage.Record, error) {
	name := fmt.Sprintf("storage-%d-%s", purpose, strings.Split(uuid.New().String(), "-")[0])
	levels := uint8(maxStorageAccountLevels)

	address, _, err := cvm.GetStorageAccountAddress(&cvm.GetMemoryAccountAddressArgs{
		Name: name,
		Vm:   vm.PublicKey().ToBytes(),
	})
	if err != nil {
		return nil, err
	}

	record := &storage.Record{
		Vm: vm.PublicKey().ToBase58(),

		Name:              name,
		Address:           base58.Encode(address),
		Levels:            levels,
		AvailableCapacity: storage.GetMaxCapacity(levels),
		Purpose:           purpose,
	}

	txn := solana.NewTransaction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		compute_budget.SetComputeUnitLimit(100_000),
		compute_budget.SetComputeUnitPrice(10_000),
		cvm.NewVmStorageInitInstruction(
			&cvm.VmStorageInitInstructionAccounts{
				VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
				Vm:          vm.PublicKey().ToBytes(),
				VmStorage:   address,
			},
			&cvm.VmStorageInitInstructionArgs{
				Name:   name,
				Levels: levels,
			},
		),
	)

	bh, err := p.data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	txn.SetBlockhash(bh)

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes())
	if err != nil {
		return nil, err
	}

	for i := 0; i < 30; i++ {
		sig, err := p.data.SubmitBlockchainTransaction(ctx, &txn)
		if err != nil {
			return nil, err
		}

		time.Sleep(4 * time.Second)

		finalizedTxn, err := p.data.GetBlockchainTransaction(ctx, base58.Encode(sig[:]), solana.CommitmentFinalized)
		if err == nil && finalizedTxn.Err == nil && finalizedTxn.Meta.Err == nil {
			return record, nil
		}
	}

	return nil, errors.New("txn did not finalize")
}
