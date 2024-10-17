package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/treasury"
	"github.com/code-payments/code-server/pkg/code/data/treasury/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_treasurypool(
			id SERIAL NOT NULL PRIMARY KEY,

			vm TEXT NOT NULL,

			name TEXT NOT NULL,

			address TEXT NOT NULL,
			bump INTEGER NOT NULL,

			vault TEXT NOT NULL,
			vault_bump INTEGER NOT NULL,

			authority TEXT NOT NULL,

			merkle_tree_levels INTEGER NOT NULL,

			current_index INTEGER NOT NULL,
			history_list_size INTEGER NOT NULL,

			solana_block INTEGER NOT NULL,

			state INTEGER NOT NULL,
			
			last_updated_at TIMESTAMP WITH TIME ZONE,

			CONSTRAINT codewallet__core_treasurypool__uniq__name UNIQUE (name),
			CONSTRAINT codewallet__core_treasurypool__uniq__address UNIQUE (address),
			CONSTRAINT codewallet__core_treasurypool__uniq__vault UNIQUE (vault)
		);

		CREATE TABLE codewallet__core_treasurypoolrecentroot(
			id SERIAL NOT NULL PRIMARY KEY,

			pool TEXT NOT NULL,
			index INTEGER NOT NULL,
			recent_root TEXT NOT NULL,
			at_solana_block INTEGER NOT NULL,

			CONSTRAINT codewallet__core_treasurypoolrecentroot__uniq__pool__and__index__and__at_solana_block UNIQUE (pool, index, at_solana_block)
		);

		CREATE TABLE codewallet__core_treasurypoolfunding(
			id SERIAL NOT NULL PRIMARY KEY,

			vault TEXT NOT NULL,
			delta_quarks BIGINT NOT NULL,
			transaction_id TEXT NOT NULL,
			state INTEGER NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE,

			CONSTRAINT codewallet__core_treasurypoolfunding__uniq__transaction_id UNIQUE (transaction_id)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_treasurypool;
		DROP TABLE codewallet__core_treasurypoolrecentroot;
		DROP TABLE codewallet__core_treasurypoolfunding;
	`
)

var (
	testStore treasury.Store
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

func TestTreasuryPoolPostgresStore(t *testing.T) {
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
