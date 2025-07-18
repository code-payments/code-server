package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_fulfillment(
			id SERIAL NOT NULL PRIMARY KEY,

			intent TEXT NOT NULL,
			intent_type INTEGER NOT NULL,

			action_id INTEGER NOT NULL,
			action_type INTEGER NOT NULL,

			fulfillment_type INTEGER NOT NULL,
			data BYTEA NULL,
			signature TEXT NULL UNIQUE,

			nonce TEXT NULL,
			blockhash TEXT NULL,

			virtual_signature TEXT NULL UNIQUE,
			virtual_nonce TEXT NULL,
			virtual_blockhash TEXT NULL,

			source TEXT NOT NULL,
			destination TEXT NULL,

			intent_ordering_index BIGINT NOT NULL,
			action_ordering_index INTEGER NOT NULL,
			fulfillment_ordering_index INTEGER NOT NULL,

			disable_active_scheduling BOOL NOT NULL,

			state INTEGER NOT NULL,

			version INTEGER NOT NULL,

			batch_insertion_id INTEGER NOT NULL,

			created_at timestamp with time zone NOT NULL
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_fulfillment;
	`
)

var (
	testStore fulfillment.Store
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

func TestFulfillmentPostgresStore(t *testing.T) {
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
