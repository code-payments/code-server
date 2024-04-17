package async_nonce

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mr-tron/base58/base58"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/code-payments/code-server/pkg/solana"
	compute_budget "github.com/code-payments/code-server/pkg/solana/computebudget"
	"github.com/code-payments/code-server/pkg/solana/system"
)

var (
	sigTimeout      = time.Minute * 5
	sigTimeoutCache = make(map[string]time.Time) // temporary hack
	sigCacheMu      sync.Mutex
)

func (p *service) markReleased(ctx context.Context, record *nonce.Record) error {
	// We know the nonce is ready but don't know the blockhash for it.
	record.State = nonce.StateReleased
	return p.data.SaveNonce(ctx, record)
}

func (p *service) markAvailable(ctx context.Context, record *nonce.Record) error {
	// We now know the blockhash for the nonce.
	record.State = nonce.StateAvailable
	return p.data.SaveNonce(ctx, record)
}

func (p *service) markInvalid(ctx context.Context, record *nonce.Record) error {
	// We failed to create the nonce account (insufficient funds, etc).
	record.State = nonce.StateInvalid
	return p.data.SaveNonce(ctx, record)
}

func (p *service) sign(tx *solana.Transaction, key *vault.Record) error {
	priv, err := key.GetPrivateKey()
	if err != nil {
		return err
	}

	err = tx.Sign(common.GetSubsidizer().PrivateKey().ToBytes(), priv)
	if err != nil {
		return err
	}

	return nil
}

func (p *service) getLatestBlockhash(ctx context.Context) (*solana.Blockhash, error) {
	bh, err := p.data.GetBlockchainLatestBlockhash(ctx)
	if err != nil {
		return nil, err
	}

	return &bh, nil
}

func (p *service) getTransaction(ctx context.Context, signature string) (*transaction.Record, error) {
	return p.getTransactionFromBlockchain(ctx, signature)
}

func (p *service) getTransactionFromBlockchain(ctx context.Context, signature string) (*transaction.Record, error) {
	stx, err := p.data.GetBlockchainTransaction(ctx, signature, solana.CommitmentFinalized)
	if err == solana.ErrSignatureNotFound {
		return nil, transaction.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	tx, err := transaction.FromConfirmedTransaction(stx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (p *service) getRentAmount(ctx context.Context) (uint64, error) {
	if p.rent > 0 {
		return p.rent, nil
	}

	rent, err := p.data.GetBlockchainMinimumBalanceForRentExemption(ctx, system.NonceAccountSize)
	if err != nil {
		return 0, err
	}

	p.rent = rent
	return p.rent, nil
}

func (p *service) createNonce(ctx context.Context) (*nonce.Record, error) {
	err := common.EnforceMinimumSubsidizerBalance(ctx, p.data)
	if err != nil {
		return nil, err
	}

	key, err := p.getVaultKey(ctx)
	if err != nil {
		return nil, err
	}

	res := nonce.Record{
		Address:   key.PublicKey,
		Authority: common.GetSubsidizer().PublicKey().ToBase58(),
		Purpose:   nonce.PurposeClientTransaction, // todo: intelligently set a purpose
		State:     nonce.StateUnknown,
	}

	tx, err := p.createNonceAccountTx(ctx, &res)
	if err != nil {
		return nil, err
	}
	err = p.sign(tx, key)
	if err != nil {
		return nil, err
	}

	res.Signature = base58.Encode(tx.Signature())

	err = p.data.SaveNonce(ctx, &res)
	if err != nil {
		return nil, err
	}

	go p.broadcastTx(ctx, tx)

	return &res, nil
}

func (p *service) createNonceAccountTx(ctx context.Context, nonce *nonce.Record) (*solana.Transaction, error) {
	rent, err := p.getRentAmount(ctx)
	if err != nil {
		return nil, err
	}

	subPub := common.GetSubsidizer().PublicKey().ToBytes()
	noncePub, err := nonce.GetPublicKey()
	if err != nil {
		return nil, err
	}

	instructions := []solana.Instruction{
		compute_budget.SetComputeUnitLimit(10_000),
		compute_budget.SetComputeUnitPrice(10_000),
		system.CreateAccount(
			subPub,
			noncePub,
			system.SystemAccount,
			rent,
			system.NonceAccountSize,
		),
		system.InitializeNonce(
			noncePub,
			subPub,
		),
	}

	tx := solana.NewTransaction(subPub, instructions...)

	bh, err := p.getLatestBlockhash(ctx)
	if err != nil {
		return nil, err
	}
	tx.SetBlockhash(*bh)

	return &tx, nil
}

func (p *service) checkForMissingTx(ctx context.Context, nonce *nonce.Record) error {
	sigCacheMu.Lock()
	defer sigCacheMu.Unlock()

	// todo: use the DB to store the createdAt time of the nonce account

	t := sigTimeoutCache[nonce.Signature]
	if t.IsZero() {
		sigTimeoutCache[nonce.Signature] = time.Now()
		return nil
	}

	if time.Since(t) > sigTimeout {
		return errors.New("nonce signature timeout reached")
	}

	return nil
}

func (p *service) broadcastTx(ctx context.Context, tx *solana.Transaction) {
	log := p.log.WithField("method", "broadcastTx")

	_, err := p.data.SubmitBlockchainTransaction(ctx, tx)
	if err != nil {
		log.WithError(err).Warn("failure submitting transaction to blockchain")
	}

	timeoutChan := time.After(40 * time.Second)
	for {
		select {
		case <-time.After(5 * time.Second):
			_, err = p.data.SubmitBlockchainTransaction(ctx, tx)
			if err != nil {
				log.WithError(err).Warn("failure submitting transaction to blockchain")
			}
		case <-timeoutChan:
			return
		case <-ctx.Done():
			return
		}
	}
}
