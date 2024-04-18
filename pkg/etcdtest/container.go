package etcdtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	teardown = func() {}

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

func StartEtcdCluster(pool *dockertest.Pool, size int, autoRemove bool) (client *v3.Client, nodes []*dockertest.Resource, teardown func(), err error) {
	teardown = func() {}

	if size%2 == 0 {
		return nil, nil, teardown, fmt.Errorf("must specify an odd number of size")
	}

	clusterName, err := randClusterName()
	if err != nil {
		return nil, nil, teardown, fmt.Errorf("failed to generate cluster name: %w", err)
	}

	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method":  "StartEtcdCluster",
		"cluster": clusterName,
	})

	network, err := pool.CreateNetwork(fmt.Sprintf("%s-network", clusterName))
	if err != nil {
		return nil, nil, teardown, fmt.Errorf("failed to create network: %w", err)
	}

	cleanupNetwork := func() {
		if err := pool.RemoveNetwork(network); err != nil {
			log.WithError(err).Errorf("failed to remove network")
		}
	}

	teardown = func() {
		cleanupNetwork()
	}

	cluster := fmt.Sprintf("%s-0=http://%s-0:2380", clusterName, clusterName)
	for i := 1; i < size; i++ {
		cluster += fmt.Sprintf(",%s-%d=http://%s-%d:2380", clusterName, i, clusterName, i)
	}

	containers := make([]*dockertest.Resource, size)
	for i := 0; i < size; i++ {
		peerURL := fmt.Sprintf("http://%s-%d:2380", clusterName, i)
		containers[i], err = pool.RunWithOptions(&dockertest.RunOptions{
			Repository: imageName,
			Tag:        imageTag,
			Name:       fmt.Sprintf("%s-%d", clusterName, i),
			Networks:   []*dockertest.Network{network},
			Env: []string{
				"ALLOW_NONE_AUTHENTICATION=true",
				"ETCD_NAME=" + fmt.Sprintf("%s-%d", clusterName, i),
				"ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379",
				"ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379",
				"ETCD_INITIAL_CLUSTER=" + cluster,
				"ETCD_INITIAL_ADVERTISE_PEER_URLS=" + peerURL,
				"ETCD_LISTEN_PEER_URLS=http://0.0.0.0:2380",
			},
		}, func(config *docker.HostConfig) {
			config.AutoRemove = autoRemove
			config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		})

		if err != nil {
			return nil, nil, teardown, fmt.Errorf("failed to start etcd-%d: %w", i, err)
		}
	}

	teardown = func() {
		for _, c := range containers {
			if err := pool.Purge(c); err != nil {
				log.WithError(err).Errorf("failed to cleanup %s resource", c.Container.Name)
			}
		}

		cleanupNetwork()
	}

	endpoints := make([]string, size)
	for i := range endpoints {
		endpoints[i] = fmt.Sprintf("localhost:%s", containers[i].GetPort("2379/tcp"))
	}

	client, err = v3.New(v3.Config{
		Endpoints: endpoints,
	})
	if err != nil {
		return nil, nil, teardown, fmt.Errorf("failed to create v3 client: %w", err)
	}

	err = pool.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err = client.Get(ctx, "__startup_test")
		return err
	})
	if err != nil {
		return nil, nil, teardown, fmt.Errorf("failed waiting for stable connection: %w", err)
	}

	return client, containers, teardown, nil
}

func randClusterName() (string, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("etcd-test-%s", hex.EncodeToString(b[:])), nil
}
