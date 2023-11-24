package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/merkletree"
	"github.com/code-payments/code-server/pkg/code/data/merkletree/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_merkletreemetadata(
			id SERIAL NOT NULL PRIMARY KEY,

			name TEXT NOT NULL,
			levels INTEGER NOT NULL,
			next_index BIGINT NOT NULL,
			seed BYTEA NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,

			CONSTRAINT codewallet__core_merkletreemetadata__uniq__name UNIQUE (name)
		);

		CREATE TABLE codewallet__core_merkletreenodes(
			id SERIAL NOT NULL PRIMARY KEY,

			tree_id BIGINT NOT NULL,
			level INTEGER NOT NULL,
			index BIGINT NOT NULL,
			hash BYTEA NOT NULL,
			leaf_value BYTEA,
			version BIGINT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL,

			CONSTRAINT codewallet__core_merkletreenodes__uniq__tree_id__and__level__and__index__and__version UNIQUE (tree_id, level, index, version)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_merkletreemetadata;
		DROP TABLE codewallet__core_merkletreenodes;
	`
)

var (
	testStore merkletree.Store
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

func TestMerkleTreePostgresStore(t *testing.T) {
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
