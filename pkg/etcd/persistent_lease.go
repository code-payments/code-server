package etcd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	v3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

// PersistentLease is "persistent" lease in etcd.
//
// While name sounds contradictory, it's behaviour is that while the
// PersistentLease is not closed, it will continually try to keep the
// <key, value> pair present in etcd, _attached_ to a lease. This
// process will only end when Close() is called.
//
// PersistentLease's are useful in situations where we want to register
// the presence of a server/node in a cluster. The use of a lease ensures
// that if the process crashes or becomes partitioned from etcd, the
// value gets automatically removed. The 'persistent' component ensures that
// when the network partition is restored (or whatever failure mode that caused
// the lease to expire recovers), the <key, value> pair will be placed back
// automatically.
type PersistentLease struct {
	log    *logrus.Entry
	client *v3.Client
	ttl    int

	key        string
	val        string
	valCh      chan string
	recreateCh chan struct{}

	closeFn sync.Once
	closeCh chan struct{}
}

// NewPersistentLease creates a new PersistentLease which starts immediately
// in the background.
func NewPersistentLease(client *v3.Client, key, val string, ttl time.Duration) (*PersistentLease, error) {
	ttlSeconds := ttl.Truncate(time.Second).Seconds()
	if ttlSeconds < 1 || ttlSeconds > 60 { // Bounded by etcd library.
		return nil, fmt.Errorf("invalid ttl %v: must be [1, 60]", ttl)
	}

	pl := &PersistentLease{
		log: logrus.StandardLogger().WithFields(logrus.Fields{
			"type": "etcd/persistent_lease",
			"key":  key,
		}),
		client: client,
		ttl:    int(ttlSeconds),

		key:        key,
		val:        val,
		valCh:      make(chan string),
		recreateCh: make(chan struct{}),

		closeCh: make(chan struct{}),
	}

	go pl.syncLoop()
	go pl.watchExistence()

	return pl, nil
}

// SetValue sets the new value for the persistent lease.
//
// SetValue is done concurrently, so there are no guarantees for when this will propagate.
// To wait for the value to be replicated, a Get()/Watch() should be used.
func (pl *PersistentLease) SetValue(val string) {
	pl.valCh <- val
}

// Close closes the PersistentLease, terminating all background jobs.
//
// Close is idempotent. A PersistentLease cannot be restarted, however.
func (pl *PersistentLease) Close() {
	pl.closeFn.Do(func() {
		close(pl.closeCh)
	})
}

// syncLoop ensures that we always have an active session, and that our
// value is up-to-date.
//
// The loop re-runs on session loss, SetValue() emitting a value, or if
// watchExistence() detects a loss.
func (pl *PersistentLease) syncLoop() {
	_, _ = retry.Retry(
		func() error {
			session, err := concurrency.NewSession(pl.client, concurrency.WithTTL(pl.ttl))
			if err != nil {
				return err
			}
			defer func() {
				if err := session.Close(); err != nil {
					pl.log.WithError(err).Warn("Failed to close session")
				}
			}()

			for {
				ctx, cancel := context.WithTimeout(context.Background(), (time.Duration(pl.ttl) * time.Second).Truncate(time.Second))
				_, err := pl.client.Put(ctx, pl.key, pl.val, v3.WithLease(session.Lease()))
				cancel()
				if err != nil {
					return fmt.Errorf("failed to write key %q: %w", pl.key, err)
				}

				select {
				case <-pl.closeCh:
					return nil
				case <-session.Done():
					return errors.New("session closed")
				case <-pl.recreateCh:
					// Our watcher has indicated that we aren't visible!
				case pl.val = <-pl.valCh:
					// Value has changed, update it
				}
			}
		},
		func(attempts uint, err error) bool {
			pl.log.WithError(err).Warn("failure in lease loop")
			return true
		},
		retry.BackoffWithJitter(backoff.Constant(time.Second), time.Second, 0.1),
	)
}

// watchExistence watches the specified key to ensure it exists.
//
// This provides resilience against external actors removing the key.
// Additionally, it helps provide some extra checks if the syncLoop()
// doesn't detect a session loss.
func (pl *PersistentLease) watchExistence() {
	_, _ = retry.Retry(
		func() error {
			select {
			case <-pl.closeCh:
				return nil
			default:
			}

			cancelledCh := make(chan struct{})
			defer close(cancelledCh)

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				select {
				case <-pl.closeCh:
				case <-cancelledCh:
				}

				cancel()
			}()

			watchCh := pl.client.Watch(ctx, pl.key)
			for w := range watchCh {
				if w.Err() != nil {
					return w.Err()
				}

				for _, e := range w.Events {
					if e.Type == v3.EventTypeDelete {
						pl.recreateCh <- struct{}{}
					}
				}
			}

			return nil
		},
		func(attempts uint, err error) bool {
			pl.log.WithError(err).Warn("failure in lease loop")
			return true
		},
		retry.BackoffWithJitter(backoff.Constant(time.Second), time.Second, 0.1),
	)
}
