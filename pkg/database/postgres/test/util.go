package test

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"github.com/pkg/errors"

	_ "github.com/jackc/pgx/v4/stdlib" //nolint:revive

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	containerName     = "postgres"
	containerVersion  = "10.4"
	containerAutoKill = 120 * time.Second

	port     = 5432
	user     = "localtest"
	password = "localpassword"
	dbname   = "testdb"
)

const (
	postgresUserEnv     = "POSTGRES_USER=" + user
	postgresPasswordEnv = "POSTGRES_PASSWORD=" + password
	postgresDbEnv       = "POSTGRES_DB=" + dbname
)

// StartPostgresDB starts a Docker container using the postgres image and returns a postgres client for testing purposes.
func StartPostgresDB(pool *dockertest.Pool) (db *sql.DB, closeFunc func(), err error) {
	closeFunc = func() {}

	// Pulls the image, creates a container based on it and runs it
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: containerName,
		Tag:        containerVersion,
		Env: []string{
			"listen_addresses = '*'",
			postgresUserEnv,
			postgresPasswordEnv,
			postgresDbEnv,
		},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})

	// Check if the container resource was generated as expected
	if err != nil {
		return nil, closeFunc, errors.Wrapf(err, "failed to start resource")
	}

	// Uncomment this to view docker container logs (note: this will fully consume os.Stdout)
	/*
		opts := docker.LogsOptions{
			Context: context.Background(),

			Stderr:      true,
			Stdout:      true,
			Follow:      true,
			Timestamps:  true,
			RawTerminal: true,

			Container: resource.Container.ID,

			OutputStream: os.Stdout,
		}
		pool.Client.Logs(opts)
	*/

	hostAndPort := resource.GetHostPort(fmt.Sprintf("%d/tcp", port))
	databaseUrl := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", user, password, hostAndPort, dbname)

	// logrus.StandardLogger().Println("Connecting to database on url: ", databaseUrl)
	// logrus.StandardLogger().Println("Setting container auto-kill to: ", containerAutoKill, " seconds")

	// Tell docker to expire the container (kill) after containerAutoKill (120 sec).
	//
	// You may need to adjust this number if it is too low for your test environment.
	//
	// 2024/04/11: Expire() _never_ returns an error.
	_ = resource.Expire(uint(containerAutoKill.Seconds()))

	_, err = retry.Retry(
		func() error {
			db, err = sql.Open("pgx", databaseUrl)
			if err != nil {
				return err
			}
			return db.Ping()
		},
		retry.Limit(50),
		retry.Backoff(backoff.Constant(500*time.Millisecond), 500*time.Second),
	)
	if err != nil {
		return nil, closeFunc, errors.Wrap(err, "timed out waiting for postgres container to become available")
	}

	return db, closeFunc, nil
}
