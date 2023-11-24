package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/timelock"
	"github.com/code-payments/code-server/pkg/code/data/timelock/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_timelock(
			id SERIAL NOT NULL PRIMARY KEY,

			data_version INTEGER NOT NULL,

			address TEXT NOT NULL,
			bump INTEGER NOT NULL,

			vault_address TEXT NOT NULL,
			vault_bump INTEGER NOT NULL,
			vault_owner TEXT NOT NULL,
			vault_state INTEGER NOT NULL,

			time_authority TEXT NOT NULL,
			close_authority TEXT NOT NULL,

			num_days_locked INTEGER NOT NULL,
			unlock_at INTEGER,

			block INTEGER NOT NULL,

			last_updated_at TIMESTAMP WITH TIME ZONE,

			CONSTRAINT codewallet__core_timelock__uniq__address UNIQUE (address),
			CONSTRAINT codewallet__core_timelock__uniq__vault_address UNIQUE (vault_address),
			CONSTRAINT codewallet__core_timelock__uniq__address__and__vault_owner UNIQUE (address, vault_owner),
			CONSTRAINT codewallet__core_timelock__uniq__address__and__vault_address UNIQUE (address, vault_address)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_timelock;
	`
)

var (
	testStore timelock.Store
	teardown  func()
)

func TestMain(m *testing.M) {
	log := logrus.StandardLogger()

	testPool, err := dockertest.NewPool("")
	if err != nil {
		log.WithError(err).Error("Error creating docker pool")
		os.Exit(1)
	}

	var cleanUpFunc func()
	db, cleanUpFunc, err := postgrestest.StartPostgresDB(testPool)
	if err != nil {
		log.WithError(err).Error("Error starting postgres image")
		os.Exit(1)
	}
	defer db.Close()

	if err := createTestTables(db); err != nil {
		logrus.StandardLogger().WithError(err).Error("Error creating test tables")
		cleanUpFunc()
		os.Exit(1)
	}

	testStore = New(db)
	teardown = func() {
		if pc := recover(); pc != nil {
			cleanUpFunc()
			panic(pc)
		}

		if err := resetTestTables(db); err != nil {
			logrus.StandardLogger().WithError(err).Error("Error resetting test tables")
			cleanUpFunc()
			os.Exit(1)
		}
	}

	code := m.Run()
	cleanUpFunc()
	os.Exit(code)
}

func TestTimelockPostgresStore(t *testing.T) {
	tests.RunTests(t, testStore, teardown)
}

func createTestTables(db *sql.DB) error {
	_, err := db.Exec(tableCreate)
	if err != nil {
		logrus.StandardLogger().WithError(err).Error("could not create test tables")
		return err
	}
	return nil
}

func resetTestTables(db *sql.DB) error {
	_, err := db.Exec(tableDestroy)
	if err != nil {
		logrus.StandardLogger().WithError(err).Error("could not drop test tables")
		return err
	}

	return createTestTables(db)
}
