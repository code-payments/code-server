package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/commitment"
	"github.com/code-payments/code-server/pkg/code/data/commitment/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_commitment(
			id SERIAL NOT NULL PRIMARY KEY,

			address TEXT NOT NULL,
			vault TEXT NOT NULL,

			pool TEXT NOT NULL,
			recent_root TEXT NOT NULL,

			transcript TEXT NOT NULL,
			destination TEXT NOT NULL,
			amount BIGINT NOT NULL CHECK (amount >= 0),

			intent TEXT NOT NULL,
			action_id INTEGER NOT NULL,

			owner TEXT NOT NULL,

			state INTEGER NOT NULL,

			treasury_repaid BOOL NOT NULL,
			repayment_diverted_to TEXT NULL,

			created_at TIMESTAMP WITH TIME ZONE NOT NULL,

			CONSTRAINT codewallet__core_commitment__uniq__address UNIQUE (address),
			CONSTRAINT codewallet__core_commitment__uniq__vault UNIQUE (vault),
			CONSTRAINT codewallet__core_commitment__uniq__transcript UNIQUE (transcript),
			CONSTRAINT codewallet__core_commitment__uniq__intent__and__action_id UNIQUE (intent, action_id)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_commitment;
	`
)

var (
	testStore commitment.Store
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

func TestCommitmentPostgresStore(t *testing.T) {
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
