package etcdtest

import (
	"context"
	"fmt"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/sirupsen/logrus"
	v3 "go.etcd.io/etcd/client/v3"
)

const (
	imageName = "quay.io/coreos/etcd"
	imageTag  = "v3.5.13"

	containerAutoKill = 120 * time.Second
)

func StartEtcd(pool *dockertest.Pool) (client *v3.Client, teardown func(), err error) {
	teardown = func() {
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: imageName,
		Tag:        imageTag,
		Env: []string{
			"ALLOW_NONE_AUTHENTICATION=true",
			"ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379",
			"ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379",
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})

	if err != nil {
		return nil, teardown, fmt.Errorf("failed to start etcd: %w", err)
	}

	// 2024/04/14: Expire() _never_ returns an error
	_ = resource.Expire(uint(containerAutoKill.Seconds()))

	log := logrus.StandardLogger().WithField("method", "StartEtcd")

	client, err = v3.New(v3.Config{
		Endpoints: []string{fmt.Sprintf("localhost:%s", resource.GetPort("2379/tcp"))},
	})
	if err != nil {
		return nil, teardown, fmt.Errorf("failed to create v3 client: %w", err)
	}

	teardown = func() {
		if err := pool.Purge(resource); err != nil {
			log.WithError(err).Errorf("failed to cleanup etcd resource")
		}
	}

	err = pool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err = client.Get(ctx, "__startup_test")
		return err
	})
	if err != nil {
		return nil, teardown, fmt.Errorf("failed waiting for stable connection: %w", err)
	}

	return client, teardown, nil
}
