package async_nonce

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/nonce"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

var (
	ErrInvalidNonceAccountSize   = errors.New("invalid nonce account size")
	ErrInvalidNonceLimitExceeded = errors.New("nonce account limit exceeded")
	ErrNoAvailableKeys           = errors.New("no available keys in the vault")
)

const (
	nonceBatchSize = 100

	noncePoolSizeDefault  = 10 // Reserve is calculated as size * 2
	nonceKeyPrefixDefault = "non"

	nonceKeyPrefixEnv = "NONCE_PUBKEY_PREFIX"
	noncePoolSizeEnv  = "NONCE_POOL_SIZE"
)

type service struct {
	log  *logrus.Entry
	data code_data.Provider

	rent   uint64
	prefix string
	size   int
}

func New(data code_data.Provider) async.Service {
	return &service{
		log:    logrus.StandardLogger().WithField("service", "nonce"),
		data:   data,
		prefix: nonceKeyPrefixDefault,
		size:   noncePoolSizeDefault,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	// Look for user defined prefix value
	prefix := os.Getenv(nonceKeyPrefixEnv)
	if len(prefix) > 0 {
		p.prefix = prefix
	}

	// Look for user defined pool size value
	sizeStr := os.Getenv(noncePoolSizeEnv)
	if len(sizeStr) > 0 {
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return errors.Wrap(err, "invalid nonce pool size")
		}
		p.size = size
	}

	errCh := make(chan error, 2+2+1)

	// Generate vault keys until we have at least 10 in reserve to use for the pool
	go func() {
		if err := p.generateKeys(ctx); err != nil {
			errCh <- fmt.Errorf("failed to generate keys: %w", err)
		}
	}()

	// Watch the size of the nonce pool and create accounts if necessary
	go func() {
		if err := p.generateNonceAccounts(ctx); err != nil {
			errCh <- fmt.Errorf("failed to generate nonce accounts: %w", err)
		}
	}()

	// Setup workers to watch for nonce state changes on the Solana side
	for _, item := range []nonce.State{
		nonce.StateUnknown,
		nonce.StateReleased,
	} {
		go func(state nonce.State) {
			if err := p.worker(ctx, state, interval); err != nil && errors.Is(err, context.Canceled) {
				errCh <- err
			}
		}(item)
	}

	go func() {
		if err := p.metricsGaugeWorker(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
