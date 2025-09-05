package transaction

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/nonce"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	nonceMetricsStructName          = "transaction.Nonce"
	localNoncePoolMetricsStructName = "transaction.LocalNoncePool"
)

var (
	ErrNoncePoolNotFound = errors.New("nonce pool not found")
	ErrNoAvailableNonces = errors.New("no available nonces")
	ErrNoncePoolClosed   = errors.New("nonce pool is closed")
)

// NoncePoolOption configures a nonce pool.
type NoncePoolOption func(*noncePoolOpts)

type noncePoolOpts struct {
	desiredPoolSize int

	nodeID        string
	minExpiration time.Duration
	maxExpiration time.Duration

	refreshInterval     time.Duration
	refreshPoolInterval time.Duration

	shutdownGracePeriod time.Duration
}

func defaultOptions() noncePoolOpts {
	return noncePoolOpts{
		desiredPoolSize: 100,

		nodeID:        uuid.New().String(),
		minExpiration: 3 * time.Minute,
		maxExpiration: 5 * time.Minute,

		refreshInterval:     5 * time.Second,
		refreshPoolInterval: 10 * time.Second,

		shutdownGracePeriod: time.Minute,
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

// WithNoncePoolNodeID configures the node id to use when claiming nonces.
func WithNoncePoolNodeID(id string) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.nodeID = id
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

// WithNoncePoolShutdownGracePeriod configures the amount of time given for
// cleanup on call to Close().
func WithNoncePoolShutdownGracePeriod(duration time.Duration) NoncePoolOption {
	return func(npo *noncePoolOpts) {
		npo.shutdownGracePeriod = duration
	}
}

func (opts *noncePoolOpts) validate() error {
	if opts.desiredPoolSize < 10 {
		return errors.New("pool size must greater than 10")
	}

	if opts.nodeID == "" {
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

	if opts.shutdownGracePeriod < time.Second {
		return fmt.Errorf("invalid shutdown grace period %v, must be greater than 1s", opts.shutdownGracePeriod)
	}

	return nil
}

// Nonce represents a handle to a nonce that is owned by a local nonce pool.
type Nonce struct {
	Account   *common.Account
	Blockhash solana.Blockhash

	pool   *LocalNoncePool
	record *nonce.Record
}

// MarkReservedWithSignature marks the nonce as reserved with a signature
func (n *Nonce) MarkReservedWithSignature(ctx context.Context, sig string) error {
	tracer := metrics.TraceMethodCall(ctx, nonceMetricsStructName, "MarkReservedWithSignature")
	defer tracer.End()

	err := func() error {
		if len(sig) == 0 {
			return errors.New("signature is empty")
		}

		if n.record.Signature == sig {
			return nil
		}

		if n.record.State == nonce.StateReserved || len(n.record.Signature) != 0 {
			return errors.New("nonce already reserved with a different signature")
		}

		if !n.record.CanReserveWithSignature() {
			return errors.New("nonce is not in a valid state to reserve with signature")
		}

		n.record.State = nonce.StateReserved
		n.record.Signature = sig
		n.record.ClaimNodeID = nil
		n.record.ClaimExpiresAt = nil
		return n.pool.data.SaveNonce(ctx, n.record)
	}()
	tracer.OnError(err)
	return err
}

// ReleaseIfNotReserved releases the nonce back to the pool if
// the nonce has not yet been reserved (or more specifically, is
// still owned by the pool).
func (n *Nonce) ReleaseIfNotReserved(ctx context.Context) {
	tracer := metrics.TraceMethodCall(ctx, nonceMetricsStructName, "ReleaseIfNotReserved")
	defer tracer.End()

	if n.record.State != nonce.StateClaimed {
		return
	}
	if *n.record.ClaimNodeID != n.pool.opts.nodeID {
		return
	}
	if n.record.ClaimExpiresAt.Before(time.Now()) {
		return
	}

	n.pool.mu.Lock()
	n.pool.freeList = append(n.pool.freeList, n)
	n.pool.mu.Unlock()
}

// SelectNoncePool selects a nonce pool from the provided set that matches the
// desired environment and pool type.
//
// ErrNoncePoolNotFound is returned if no nonce pool matches the desired config.
func SelectNoncePool(env nonce.Environment, envInstance string, poolType nonce.Purpose, pools ...*LocalNoncePool) (*LocalNoncePool, error) {
	for _, pool := range pools {
		if pool.env == env && pool.envInstance == envInstance && pool.poolType == poolType {
			return pool, nil
		}
	}
	return nil, ErrNoncePoolNotFound
}

// LocalNoncePool is a pool of nonces that are cached in memory for
// quick access. The LocalNoncePool will continually monitor the pool
// to ensure sufficient size, as well refresh nonce expiration
// times.
//
// If the pool empties before it can be refilled, ErrNoAvailableNonces
// will be returned. Therefore, the pool should be sufficiently large
// such that the consumption of poolSize/2 nonces is _slower_ than the
// operation to top up the pool.
type LocalNoncePool struct {
	log *logrus.Entry

	data code_data.Provider

	metricsProvider *newrelic.Application

	env         nonce.Environment
	envInstance string
	poolType    nonce.Purpose

	opts noncePoolOpts

	workerCtx       context.Context
	cancelWorkerCtx context.CancelFunc

	mu       sync.RWMutex
	freeList []*Nonce
	isClosed bool

	refreshPoolCh chan struct{}
}

func NewLocalNoncePool(
	data code_data.Provider,
	metricsProvider *newrelic.Application,
	env nonce.Environment,
	envInstance string,
	poolType nonce.Purpose,
	opts ...NoncePoolOption,
) (*LocalNoncePool, error) {
	np := &LocalNoncePool{
		log: logrus.StandardLogger().WithFields(logrus.Fields{
			"type":                 "transaction/LocalNoncePool",
			"environment":          env.String(),
			"environment_instance": envInstance,
			"pool_type":            poolType.String(),
		}),

		data: data,

		metricsProvider: metricsProvider,

		env:         env,
		envInstance: envInstance,
		poolType:    poolType,

		opts: defaultOptions(),

		refreshPoolCh: make(chan struct{}),
	}

	for _, o := range opts {
		o(&np.opts)
	}
	if err := np.opts.validate(); err != nil {
		return nil, err
	}

	np.workerCtx, np.cancelWorkerCtx = context.WithCancel(context.Background())

	_, err := np.load(np.workerCtx, np.opts.desiredPoolSize)
	switch err {
	case nil, nonce.ErrNonceNotFound:
	default:
		np.cancelWorkerCtx()
		return nil, err
	}

	go np.refreshPool()
	go np.refreshNonces()
	go np.metricsPoller()

	return np, nil
}

func (np *LocalNoncePool) GetNonce(ctx context.Context) (*Nonce, error) {
	tracer := metrics.TraceMethodCall(ctx, localNoncePoolMetricsStructName, "GetNonce")
	defer tracer.End()

	n, err := func() (*Nonce, error) {
		var n *Nonce

		np.mu.Lock()
		if np.isClosed {
			np.mu.Unlock()
			return nil, ErrNoncePoolClosed
		}

		size := len(np.freeList)
		if size > 0 {
			n = np.freeList[0]
			np.freeList = np.freeList[1:]
		}
		np.mu.Unlock()

		if size < np.opts.desiredPoolSize/2 {
			select {
			case np.refreshPoolCh <- struct{}{}:
			default:
			}
		}

		if n == nil {
			return nil, ErrNoAvailableNonces
		}
		return n, nil
	}()
	tracer.OnError(err)
	return n, err
}

func (np *LocalNoncePool) Validate(
	env nonce.Environment,
	envInstance string,
	poolType nonce.Purpose,
) error {
	if np.env != env {
		return errors.Errorf("nonce pool environment must be %s", env)
	}
	if np.envInstance != envInstance {
		return errors.Errorf("nonce pool environment instance must be %s", envInstance)
	}
	if np.poolType != poolType {
		return errors.Errorf("nonce pool type must be %s", poolType)
	}
	return nil
}

func (np *LocalNoncePool) Close() error {
	log := np.log.WithField("method", "Close")

	np.mu.Lock()
	defer np.mu.Unlock()

	if np.isClosed {
		return nil
	}

	np.isClosed = true
	np.cancelWorkerCtx()

	ctx, cancel := context.WithTimeout(context.Background(), np.opts.shutdownGracePeriod)
	defer cancel()

	remaining := len(np.freeList)
	for _, n := range np.freeList {
		n.record.State = nonce.StateAvailable
		n.record.ClaimNodeID = nil
		n.record.ClaimExpiresAt = nil

		if err := np.data.SaveNonce(ctx, n.record); err != nil {
			log.WithError(err).WithField("nonce", n.record.Address).Warn("Failed to release nonce on shutdown")
		} else {
			remaining--
		}
	}

	if remaining != 0 {
		return fmt.Errorf("failed to free all nonces (%d left unfreed)", remaining)
	}

	return nil
}

func (np *LocalNoncePool) load(ctx context.Context, limit int) (int, error) {
	np.mu.Lock()
	if np.isClosed {
		np.mu.Unlock()
		return 0, nil
	}
	np.mu.Unlock()

	now := time.Now()
	records, err := np.data.BatchClaimAvailableNoncesByPurpose(
		ctx,
		np.env,
		np.envInstance,
		np.poolType,
		limit,
		np.opts.nodeID,
		now.Add(np.opts.minExpiration),
		now.Add(np.opts.maxExpiration),
	)
	if err != nil {
		return 0, err
	}

	if len(records) == 0 {
		return 0, ErrNoAvailableNonces
	}

	var newNonces []*Nonce
	for _, record := range records {
		account, err := common.NewAccountFromPublicKeyString(record.Address)
		if err != nil {
			return 0, errors.Wrap(err, "invalid address")
		}

		decodedBh, err := base58.Decode(record.Blockhash)
		if err != nil {
			return 0, errors.Wrap(err, "invalid blochash")
		}
		var bh solana.Blockhash
		copy(bh[:], decodedBh)

		newNonces = append(newNonces, &Nonce{
			Account:   account,
			Blockhash: bh,
			pool:      np,
			record:    record,
		})
	}

	np.mu.Lock()
	np.freeList = append(np.freeList, newNonces...)
	np.mu.Unlock()

	return len(newNonces), nil
}

func (np *LocalNoncePool) refreshPool() {
	log := np.log.WithField("method", "refreshPool")

	for {
		select {
		case <-np.workerCtx.Done():
			return
		case <-np.refreshPoolCh:
		case <-time.After(np.opts.refreshPoolInterval):
		}

		np.mu.Lock()
		size := len(np.freeList)
		np.mu.Unlock()

		if size >= np.opts.desiredPoolSize {
			continue
		}

		limit := np.opts.desiredPoolSize - size
		log := log.WithField("limit", limit)
		log.Debug("Refreshing nonce pool")
		loaded, err := np.load(np.workerCtx, limit)
		if err != nil {
			log.WithError(err).Warn("Failed to refresh nonce pool")
		} else if loaded < limit {
			log.WithField("count", loaded).Warn("Unable to refresh nonce pool with desired number of nonces")
		} else {
			log.WithField("count", loaded).Debug("Nonce pool refreshed")
		}
	}
}

func (np *LocalNoncePool) refreshNonces() {
	for {
		select {
		case <-np.workerCtx.Done():
			return
		case <-time.After(np.opts.refreshInterval):
		}

		np.refreshNoncesNow()
	}
}

func (np *LocalNoncePool) refreshNoncesNow() {
	log := np.log.WithField("method", "refreshNoncesNow")

	now := time.Now()
	refreshList := make([]*Nonce, 0)

	np.mu.Lock()
	if np.isClosed {
		np.mu.Unlock()
		return
	}

	for i := 0; i < len(np.freeList); {
		n := np.freeList[i]
		if n.record.ClaimExpiresAt.Sub(now) > 2*np.opts.minExpiration/3 {
			i++
			continue
		}

		refreshList = append(refreshList, n)
		np.freeList = slices.Delete(np.freeList, i, i+1)
	}
	np.mu.Unlock()

	if len(refreshList) == 0 {
		return
	}

	log.WithField("count", len(refreshList)).Debug("Refreshing nonces")

	for _, n := range refreshList {
		log := log.WithField("nonce", n.record.Address)
		log.Debug("Refreshing nonce")

		if time.Since(*n.record.ClaimExpiresAt) >= 0 {
			log.Warn("Nonce claim is expired, abandoning")
			continue
		}
		if time.Since(*n.record.ClaimExpiresAt) > -time.Second {
			log.Warn("Nonce claim is too close to expiry, abandoning")
			continue
		}

		n.record.ClaimExpiresAt = pointer.Time(n.record.ClaimExpiresAt.Add(np.opts.minExpiration))
		err := np.data.SaveNonce(np.workerCtx, n.record)
		if err != nil {
			log.WithError(err).Warn("Failed to refresh nonce, abandoning")
		} else {
			np.mu.Lock()
			np.freeList = append(np.freeList, n)
			np.mu.Unlock()
		}
	}
}

func (np *LocalNoncePool) metricsPoller() {
	for {
		select {
		case <-np.workerCtx.Done():
			return
		case <-time.After(time.Second):
			np.recordPoolSizeMetricEvent()
		}
	}
}

func (np *LocalNoncePool) recordPoolSizeMetricEvent() {
	np.mu.Lock()
	if np.isClosed {
		np.mu.Unlock()
		return
	}
	size := len(np.freeList)
	np.mu.Unlock()

	kvs := np.getBaseMetricKvs()
	kvs["current_nonce_pool_size"] = size
	kvs["desired_nonce_pool_size"] = np.opts.desiredPoolSize

	np.metricsProvider.RecordCustomEvent("LocalNoncePoolSizePollingCheck", kvs)
}

func (np *LocalNoncePool) getBaseMetricKvs() map[string]interface{} {
	return map[string]interface{}{
		"node_id":            np.opts.nodeID,
		"nonce_env":          np.env.String(),
		"nonce_env_instance": np.envInstance,
		"nonce_pool_type":    np.poolType.String(),
	}
}
