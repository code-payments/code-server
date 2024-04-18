package etcd

import (
	"context"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/lock"
	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/api/v3/mvccpb"
	v3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type LockManager struct {
	log     *logrus.Entry
	client  *v3.Client
	rootKey string
	lockTTL int
	lockVal string

	closeOnce sync.Once
	closeCh   chan struct{}

	sessionMu sync.Mutex
	session   *concurrency.Session
}

func NewLockManager(
	client *v3.Client,
	rootKey string,
	lockTTL time.Duration,
	lockValue string,
) (*LockManager, error) {
	// We safety bound the TTL for locks to be within reason.
	//
	// WithTTL() will default the TTL to 60 seconds if TTL <= 0 || TTL > 60 seconds.
	if lockTTL < time.Second || lockTTL > time.Minute {
		return nil, fmt.Errorf("invalid lock ttl: %d (must be [1s, 60s]", lockTTL)
	}

	lockTTLSeconds := int(lockTTL.Round(time.Second).Seconds())

	session, err := concurrency.NewSession(
		client,
		concurrency.WithTTL(lockTTLSeconds),
		concurrency.WithContext(v3.WithRequireLeader(context.Background())),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd session: %w", err)
	}

	lm := &LockManager{
		log: logrus.StandardLogger().WithFields(logrus.Fields{
			"type": "etcd/LockManager",
			"root": rootKey,
		}),
		client:  client,
		rootKey: rootKey,
		lockTTL: lockTTLSeconds,
		lockVal: lockValue,

		closeCh: make(chan struct{}),
		session: session,
	}

	// The underlying session should attempt to keep itself alive forever, surviving leadership changes and
	// other temporary issues. However, it is possible for the session to enter a terminal state if there
	// are unexpected errors from the service. Notably, if the cluster ends up in a leaderless state, or
	// our configured client cannot re-establish a connection.
	//
	// In these cases, we expect manual intervention on the etcd cluster to rectify the state, at which
	// point we the rest of our services to auto recover. Quis custodiet ipsos custodes? It is watchSession()!
	go lm.watchSession()

	return lm, nil
}

// Create implements lock.DistributedLock.
func (lm *LockManager) Create(_ context.Context, name string) (lock.DistributedLock, error) {
	key := path.Join(lm.rootKey, name)

	lm.sessionMu.Lock()
	defer lm.sessionMu.Unlock()

	if lm.session == nil {
		return nil, fmt.Errorf("LockManager is closed")
	}

	return newLock(lm, key, lm.lockVal), nil
}

// Close will close the lock manager, _and all locked locks created by the lock manager will become unlocked_.
func (lm *LockManager) Close() {
	lm.closeOnce.Do(func() {
		lm.sessionMu.Lock()
		defer lm.sessionMu.Unlock()

		close(lm.closeCh)

		// Close() will cause all of the locked locks created by the session to become unlocked.
		if err := lm.session.Close(); err != nil {
			lm.log.WithError(err).Warn("failed to close etcd session on close")
		}

		lm.session = nil
	})
}

func (lm *LockManager) watchSession() {
	for {
		lm.sessionMu.Lock()
		session := lm.session
		lm.sessionMu.Unlock()

		if session == nil {
			return
		}

		select {
		case <-lm.closeCh:
			if err := session.Close(); err != nil {
				lm.log.WithError(err).Warn("failed to close etcd session on close")
			}

			return
		case <-session.Done():
		}

		lm.log.Info("Locker session expired. Attempting to recreate session...")

		session, err := concurrency.NewSession(
			lm.client,
			concurrency.WithTTL(lm.lockTTL),
			concurrency.WithContext(v3.WithRequireLeader(context.Background())),
		)
		if err != nil {
			lm.log.WithError(err).Warn("failed to recreate session for locker, retrying in 1s")
			time.Sleep(1 * time.Second)
			continue
		}

		lm.sessionMu.Lock()
		lm.session = session
		lm.sessionMu.Unlock()
	}
}

type Lock struct {
	log *logrus.Entry
	lm  *LockManager
	key string
	val string

	electionMu sync.Mutex
	election   *concurrency.Election

	resignFn func(ctx context.Context) error
}

func newLock(lm *LockManager, key, val string) *Lock {
	return &Lock{
		log: logrus.WithFields(logrus.Fields{
			"type": "etcd/Lock",
			"key":  key,
		}),
		lm:  lm,
		key: key,
		val: val,
	}
}

// Acquire implements lock.DistributedLock.
func (l *Lock) Acquire(ctx context.Context) (<-chan struct{}, error) {
	lostCh := make(chan struct{})

	l.electionMu.Lock()
	defer l.electionMu.Unlock()

	if l.election != nil {
		return nil, fmt.Errorf("cannot call Acquire concurrently")
	}

	l.lm.sessionMu.Lock()
	session := l.lm.session
	l.lm.sessionMu.Unlock()

	if session == nil {
		return nil, fmt.Errorf("lock manager closed")
	}

	campaignCtx, cancelCampaign := context.WithCancel(ctx)
	election := concurrency.NewElection(session, l.key)
	if err := election.Campaign(campaignCtx, l.val); err != nil {
		cancelCampaign()
		return lostCh, fmt.Errorf("failed to initiate lock: %w", err)
	}

	l.log.Debug("Lock acquired")
	l.election = election

	watchCh := session.Client().Watch(
		v3.WithRequireLeader(campaignCtx),
		election.Key(),
		v3.WithRev(election.Rev()),
	)

	go func() {
		defer cancelCampaign()
		defer func() {
			l.electionMu.Lock()
			defer l.electionMu.Unlock()

			if l.election == nil {
				return
			}

			if err := election.Resign(ctx); err != nil {
				l.log.WithError(err).Warn("Failed to resign on lock cleanup")
			}

			l.election = nil
		}()

		// Note: we close lostCh _before_ executing the rest of the clean-up to ensure we
		// propagate the lost notification ASAP. In particular, this is important in a case
		// where the etcd leader has been lost (and detected). In this case, Resign() will
		// not succeed until the leader has been restored.
		defer close(lostCh)

		for {
			select {
			case <-session.Done():
				l.log.Warn("Session closed/ended, releasing lock")
				return

			case watchEvent, ok := <-watchCh:
				if !ok {
					return
				}

				if err := watchEvent.Err(); err != nil {
					l.log.WithError(err).Warn("Failure watching our lock key")
					return
				}

				for _, event := range watchEvent.Events {
					switch event.Type {
					case mvccpb.PUT:
						if event.Kv.CreateRevision != election.Rev() {
							l.log.Warn("Lock key create revision changed, cowardly unlocking")
							return
						}
					case mvccpb.DELETE:
						l.log.Trace("Lock key has been removed")
						return
					}
				}
			}
		}
	}()

	return lostCh, nil
}

// Unlock implements lock.DistributedLock
func (l *Lock) Unlock(ctx context.Context) error {
	l.electionMu.Lock()
	defer l.electionMu.Unlock()

	if l.election == nil {
		return nil
	}

	err := l.election.Resign(ctx)
	l.election = nil
	return err
}

// IsLocked implements lock.DistributedLock
func (l *Lock) IsLocked() bool {
	l.electionMu.Lock()
	defer l.electionMu.Unlock()

	return l.election != nil && l.election.Key() != ""
}
