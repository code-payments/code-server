package etcd

import (
	"context"

	v3 "go.etcd.io/etcd/client/v3"
)

func WaitFor(ctx context.Context, client *v3.Client, key string, exists bool) error {
	get, err := client.Get(ctx, key)
	if err != nil {
		return err
	}

	if len(get.Kvs) == 0 && !exists {
		return nil
	} else if len(get.Kvs) > 0 && exists {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	watch := client.Watch(ctx, key, v3.WithRev(get.Header.Revision+1))
	for w := range watch {
		if err = w.Err(); err != nil {
			return err
		}

		for _, e := range w.Events {
			if e.Type == v3.EventTypePut && exists {
				return nil
			}
			if e.Type == v3.EventTypeDelete && !exists {
				return nil
			}
		}
	}

	return ctx.Err()
}
