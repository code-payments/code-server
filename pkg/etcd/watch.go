package etcd

import (
	"context"
	"github.com/code-payments/code-server/pkg/retry/backoff"
	"maps"
	"time"

	"github.com/sirupsen/logrus"
	v3 "go.etcd.io/etcd/client/v3"

	"github.com/code-payments/code-server/pkg/retry"
)

// Snapshot contains a "tree" at a given point in time.
//
// The tree is all <Key, Value> pairs matching a prefix.
type Snapshot[K comparable, V any] struct {
	Tree map[K]V
}

// KVTransform transforms a <Key, Value> pair from etcd into a
// <K, V> mapping suitable to the use-case of Snapshot[K, V].
//
// Example: K is the ID of an item, where V is the information about it.
type KVTransform[K comparable, V any] func(k, v []byte) (K, V, error)

// WatchPrefix watches a prefix _forever_ until the provided context
// is cancelled.
//
// The returned channel emits Snapshot's of the tree.
// The returned channel closes when the provided ctx is cancelled.
func WatchPrefix[K comparable, V any](
	ctx context.Context,
	client *v3.Client,
	prefix string,
	transform KVTransform[K, V],
) <-chan Snapshot[K, V] {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "WatchPrefix",
		"prefix": prefix,
	})

	ch := make(chan Snapshot[K, V], 1)

	loop := func() error {
		get, err := client.Get(ctx, prefix, v3.WithPrefix())
		if err != nil {
			return err
		}

		if get.More {
			log.WithFields(logrus.Fields{
				"total":    get.Count,
				"returned": len(get.Kvs),
			})
		}

		tree := make(map[K]V)
		for i := range get.Kvs {
			key, val, err := transform(get.Kvs[i].Key, get.Kvs[i].Value)
			if err != nil {
				log.WithError(err).
					WithField("key", string(get.Kvs[i].Key)).
					Warn("Invalid record, dropping")

				continue
			}

			tree[key] = val
		}

		ch <- Snapshot[K, V]{Tree: maps.Clone(tree)}

		watchCh := client.Watch(
			ctx,
			prefix,
			v3.WithPrefix(),
			v3.WithRev(get.Header.Revision+1),
			v3.WithPrevKV(), // Note: Important for delete case.
		)

		for watch := range watchCh {
			if err := watch.Err(); err != nil {
				return err
			}

			for _, event := range watch.Events {
				switch event.Type {
				case v3.EventTypePut:
					key, val, err := transform(event.Kv.Key, event.Kv.Value)
					if err != nil {
						log.WithError(err).
							WithField("key", string(event.Kv.Key)).
							Warn("Invalid record, dropping")

						continue
					}

					tree[key] = val
				case v3.EventTypeDelete:
					key, _, err := transform(event.PrevKv.Key, event.PrevKv.Value)
					if err != nil {
						log.WithError(err).
							WithField("key", string(event.Kv.Key)).
							Warn("Invalid record, dropping (drop)")

						continue
					}

					delete(tree, key)
				}
			}

			ch <- Snapshot[K, V]{Tree: maps.Clone(tree)}
		}

		return nil
	}

	go func() {
		_, _ = retry.Retry(
			loop,
			func(attempts uint, err error) bool {
				log.WithError(err).Warn("Failure during watch loop")
				return true
			},
			retry.NonRetriableErrors(context.Canceled),
			retry.BackoffWithJitter(backoff.Constant(time.Second), 2*time.Second, 0.1),
		)
		log.Info("Closed")
		close(ch)
	}()

	return ch
}
