package transaction

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
)

// NoncePoolOption configures a nonce pool.
type NoncePoolOption func(*noncePoolOpts)

// WithNoncePoolClock configures the clock for the nonce pool.
func WithNoncePoolClock(clock clockwork.Clock) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.clock = clock
	}
}

// WithNoncePoolSize configures the desired size of the pool.
//
// The pool will use this size to determine how much to load
// when the pool starts up, or when the pool gets low. It can
// be viewed as 'target memory' in a GC, with the actual pool
// size behaving like a saw-tooth graph.
//
// The pool does not have any mechanism to shrink the pool to
// this size beyond the natural consumption of nonces.
func WithNoncePoolSize(size int) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.desiredPoolSize = size
	}
}

// WithNoncePoolNodeId configures the node id to use when claiming nonces.
func WithNoncePoolNodeId(id string) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.nodeId = id
	}
}

// WithNoncePoolMinExpiration configures the lower bound for the
// expiration window of claimed nonces.
func WithNoncePoolMinExpiration(d time.Duration) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.minExpiration = d
	}
}

// WithNoncePoolMaxExpiration configures the upper bound for the
// expiration window of claimed nonces.
func WithNoncePoolMaxExpiration(d time.Duration) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.maxExpiration = d
	}
}

// WithNoncePoolRefreshInterval specifies how often the pool should be
// scanning it's free list for refresh candidates. Candidates are claimed
// nonces whose expiration is <= 2/3 of the min expiration.
func WithNoncePoolRefreshInterval(interval time.Duration) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.refreshInterval = interval
	}
}

// WithNoncePoolRefreshPoolInterval configures the pool to refresh the set of
// nonces at this duration. If the pool size is 1/2 the desired size, nonces
// will be fetched from DB. This condition is also checked (asynchronously) every
// time a nonce is pulled from the pool.
func WithNoncePoolRefreshPoolInterval(interval time.Duration) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.refreshPoolInterval = interval
	}
}

type noncePoolOpts struct {
	clock           clockwork.Clock
	desiredPoolSize int

	nodeId        string
	minExpiration time.Duration
	maxExpiration time.Duration

	refreshInterval     time.Duration
	refreshPoolInterval time.Duration
}

func (opts *noncePoolOpts) validate() error {
	if opts.clock == nil {
		return errors.New("missing clock")
	}
	if opts.desiredPoolSize < 10 {
		return errors.New("pool size must greater than 10")
	}
	if opts.nodeId == "" {
		return errors.New("missing node id")
	}

	if opts.minExpiration < 10*time.Second {
		return errors.New("min expiry must be >= 10s")
	}
	if opts.maxExpiration < 10*time.Second {
		return errors.New("max expiry must be >= 10s")
	}
	if opts.minExpiration > opts.maxExpiration {
		return errors.New("min expiry must <= max expiry")
	}

	if opts.refreshInterval < time.Second || opts.refreshInterval > opts.minExpiration/2 {
		return fmt.Errorf("invalid refresh interval %v, must be between (1, minExpiration/2)", opts.refreshInterval)
	}

	if opts.refreshPoolInterval < time.Second {
		return fmt.Errorf("invalid refresh pool interval %v, must be greater than 1s", opts.refreshPoolInterval)
	}

	return nil
}

// Nonce represents a handle to a nonce that is owned by a pool.
type Nonce struct {
	pool   *NoncePool
	record *nonce.Record
}

// MarkReservedWithSignature marks the nonce as reserved with a signature
func (n *Nonce) MarkReservedWithSignature(ctx context.Context, sig string) error {
	if len(sig) == 0 {
		return errors.New("signature is empty")
	}

	if n.record.Signature == sig {
		return nil
	}

	if len(n.record.Signature) != 0 {
		return errors.New("nonce already has a different signature")
	}

	// Nonce is reserved without a signature, so update its signature
	if n.record.State == nonce.StateReserved {
		n.record.Signature = sig
		return n.pool.data.SaveNonce(ctx, n.record)
	}

	if !n.record.IsAvailable() {
		return errors.New("nonce must be available to reserve")
	}

	n.record.State = nonce.StateReserved
	n.record.Signature = sig
	n.record.ClaimNodeId = ""
	n.record.ClaimExpiresAt = time.UnixMilli(0)

	return n.pool.data.SaveNonce(ctx, n.record)
}

// UpdateSignature updates the signature for a reserved nonce. The use case here
// being transactions that share a nonce, and the new transaction being designated
// as the one to submit to the blockchain.
func (n *Nonce) UpdateSignature(ctx context.Context, sig string) error {
	if len(sig) == 0 {
		return errors.New("signature is empty")
	}

	if n.record.Signature == sig {
		return nil
	}
	if n.record.State != nonce.StateReserved {
		return errors.New("nonce must be in a reserved state")
	}

	n.record.Signature = sig
	return n.pool.data.SaveNonce(ctx, n.record)
}

// ReleaseIfNotReserved releases the nonce back to the pool if
// the nonce has not yet been reserved (or more specifically, is
// still owned by the pool).
func (n *Nonce) ReleaseIfNotReserved() {
	if n.record.State != nonce.StateClaimed {
		return
	}
	if n.record.ClaimNodeId != n.pool.opts.nodeId {
		return
	}
	if n.record.ClaimExpiresAt.Before(n.pool.opts.clock.Now()) {
		return
	}

	n.pool.freeListMu.Lock()
	n.pool.freeList = append(n.pool.freeList, n)
	n.pool.freeListMu.Unlock()
}

// NoncePool is a pool of nonces that are cached in memory for
// quick access. The NoncePool will continually monitor the pool
// to ensure sufficient size, as well refresh nonce expiration
// times.
//
// If the pool empties before it can be refilled, ErrNoAvailableNonces
// will be returned. Therefore, the pool should be sufficiently large
// such that the consumption of poolSize/2 nonces is _slower_ than the
// operation to top up the pool.
type NoncePool struct {
	log      *logrus.Entry
	data     code_data.Provider
	poolType nonce.Purpose
	opts     noncePoolOpts

	ctx    context.Context
	cancel context.CancelFunc

	freeListMu sync.RWMutex
	freeList   []*Nonce

	refreshPoolCh chan struct{}
}

func NewNoncePool(
	data code_data.Provider,
	poolType nonce.Purpose,
	opts ...NoncePoolOption,
) (*NoncePool, error) {
	np := &NoncePool{
		log: logrus.StandardLogger().WithFields(logrus.Fields{
			"type":      "transaction/NoncePool",
			"pool_type": poolType.String(),
		}),
		data:     data,
		poolType: poolType,
		opts: noncePoolOpts{
			clock:               clockwork.NewRealClock(),
			desiredPoolSize:     100,
			nodeId:              uuid.New().String(),
			minExpiration:       time.Minute,
			maxExpiration:       2 * time.Minute,
			refreshInterval:     5 * time.Second,
			refreshPoolInterval: 10 * time.Second,
		},
		refreshPoolCh: make(chan struct{}, 1),
	}

	for _, o := range opts {
		o(&np.opts)
	}

	if err := np.opts.validate(); err != nil {
		return nil, err
	}

	np.ctx, np.cancel = context.WithCancel(context.Background())

	go np.refreshPool()
	go np.refreshNonces()

	return np, nil
}

func (np *NoncePool) GetNonce(ctx context.Context) (*Nonce, error) {
	var n *Nonce

	np.freeListMu.Lock()
	size := len(np.freeList)
	if size > 0 {
		n = np.freeList[0]
		np.freeList = np.freeList[1:]
	}
	np.freeListMu.Unlock()

	if n == nil {
		return nil, ErrNoAvailableNonces
	}

	if (size - 1) < np.opts.desiredPoolSize/2 {
		select {
		case np.refreshPoolCh <- struct{}{}:
		default:
		}
	}

	return n, nil
}

func (np *NoncePool) Load(ctx context.Context, limit int) error {
	now := np.opts.clock.Now()
	records, err := np.data.BatchClaimAvailableByPurpose(
		ctx,
		np.poolType,
		limit,
		np.opts.nodeId,
		now.Add(np.opts.minExpiration),
		now.Add(np.opts.maxExpiration),
	)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return ErrNoAvailableNonces
	}

	np.freeListMu.Lock()
	for i := range records {
		np.freeList = append(np.freeList, &Nonce{pool: np, record: records[i]})
	}
	np.freeListMu.Unlock()

	return nil
}

func (np *NoncePool) Close() error {
	np.freeListMu.Lock()
	defer np.freeListMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	remaining := len(np.freeList)
	for _, n := range np.freeList {
		n.record.State = nonce.StateAvailable
		n.record.ClaimNodeId = ""
		n.record.ClaimExpiresAt = time.UnixMilli(0)

		if err := np.data.SaveNonce(ctx, n.record); err != nil {
			np.log.WithError(err).WithField("nonce", n.record.Address).Warn("Failed to release nonce on shutdown")
		} else {
			remaining--
		}
	}

	if remaining != 0 {
		return fmt.Errorf("failed to free all nonces (%d left unfreed)", remaining)
	}

	np.cancel()

	return nil
}

func (np *NoncePool) refreshPool() {
	for {
		select {
		case <-np.ctx.Done():
			return
		case <-np.refreshPoolCh:
		case <-np.opts.clock.After(np.opts.refreshPoolInterval):
		}

		np.freeListMu.Lock()
		size := len(np.freeList)
		np.freeListMu.Unlock()

		if size >= np.opts.desiredPoolSize {
			continue
		}

		err := np.Load(np.ctx, np.opts.desiredPoolSize-size)
		if err != nil {
			np.log.WithError(err).Warn("Failed to refresh nonce pool")
		}
	}
}

func (np *NoncePool) refreshNonces() {
	for {
		select {
		case <-np.ctx.Done():
			return
		case <-np.opts.clock.After(np.opts.refreshInterval):
		}

		now := np.opts.clock.Now()
		refreshList := make([]*Nonce, 0)

		np.freeListMu.Lock()
		for i := 0; i < len(np.freeList); {
			n := np.freeList[i]
			if now.Sub(n.record.ClaimExpiresAt) > 2*np.opts.minExpiration/3 {
				i++
				continue
			}

			refreshList = append(refreshList, n)
			np.freeList = slices.Delete(np.freeList, i, i+1)
		}
		np.freeListMu.Unlock()

		if len(refreshList) == 0 {
			continue
		}

		for _, n := range refreshList {
			n.record.ClaimExpiresAt = n.record.ClaimExpiresAt.Add(np.opts.minExpiration)
			err := np.data.SaveNonce(np.ctx, n.record)
			if err != nil {
				np.log.WithError(err).WithField("nonce", n.record.Address).
					Warn("Failed to refresh nonce, abandoning")
			} else {
				np.freeListMu.Lock()
				np.freeList = append(np.freeList, n)
				np.freeListMu.Unlock()
			}
		}
	}
}
