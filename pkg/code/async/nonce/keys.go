package async_nonce

import (
	"context"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/code/data/vault"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func (p *service) generateKey(ctx context.Context) (*vault.Record, error) {
	// todo: audit whether we should be creating keys on the same server.
	// Perhaps this should be done outside this box.

	// Grind for a vanity key (slow)
	key, err := vault.GrindKey(p.prefix)
	if err != nil {
		return nil, err
	}
	key.State = vault.StateAvailable

	err = p.data.SaveKey(ctx, key)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func (p *service) generateKeys(ctx context.Context) error {
	err := retry.Loop(
		func() (err error) {
			// Give the server some time to breath.
			time.Sleep(time.Second * 15)

			nr := ctx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__nonce_service__vault_keys")
			defer func() {
				m.End()
				*m = newrelic.Transaction{}
			}()

			res, err := p.data.GetKeyCountByState(ctx, vault.StateAvailable)
			if err != nil {
				return err
			}

			reserveSize := (uint64(p.size) * 2)

			// If we have sufficient keys, don't generate any more.
			if res >= reserveSize {
				return nil
			}

			missing := reserveSize - res

			// Clamp the maximum number of keys to create in one run.
			if missing > 5 {
				missing = 5
			}

			p.log.Warnf("Not enough reserve keys available, generating %d more.", missing)

			// We don't have enough in the reserve, so we need to generate some.
			for i := 0; i < int(missing); i++ {
				key, err := p.generateKey(ctx)
				if err != nil {
					p.log.Error(err)
					continue
				}
				p.log.Warnf("key: %s", key.PublicKey)
			}

			return nil
		},
		retry.NonRetriableErrors(context.Canceled, ErrInvalidNonceLimitExceeded),
	)

	return err
}

func (p *service) reserveExistingKey(ctx context.Context) (*vault.Record, error) {
	// todo: add distributed locking here.

	keys, err := p.data.GetAllKeysByState(ctx, vault.StateAvailable,
		query.WithLimit(1),
	)
	if err != nil {
		return nil, err
	}

	res := keys[0]
	res.State = vault.StateReserved

	err = p.data.SaveKey(ctx, res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (p *service) getVaultKey(ctx context.Context) (*vault.Record, error) {
	key, err := p.reserveExistingKey(ctx)
	if err == nil {
		return key, nil
	}

	return nil, ErrNoAvailableKeys
}
