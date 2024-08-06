package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/code/data/vm/ram/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var (
	testStore ram.Store
	teardown  func()
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_vmmemoryaccount (
			id SERIAL NOT NULL PRIMARY KEY,

			vm TEXT NOT NULL,

			address TEXT NOT NULL,

			capacity INTEGER NOT NULL,
			num_sectors INTEGER NOT NULL,
			num_pages INTEGER NOT NULL,
			page_size INTEGER NOT NULL,

			stored_account_type INTEGER NOT NULL,

			created_at TIMESTAMP WITH TIME ZONE NOT NULL,

			CONSTRAINT codewallet__core_vmmemoryaccount__uniq__address UNIQUE (address)
		);

		CREATE TABLE codewallet__core_vmmemoryallocatedmemory (
			id SERIAL NOT NULL PRIMARY KEY,

			vm TEXT NOT NULL,

			memory_account TEXT NOT NULL,
			index INTEGER NOT NULL,
			is_allocated BOOL NOT NULL,
			stored_account_type INTEGER NOT NULL,

			last_updated_at TIMESTAMP WITH TIME ZONE NOT NULL,

			CONSTRAINT codewallet__core_vmmemoryallocatedmemory__uniq__memory_account__and__index UNIQUE (memory_account, index)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_vmmemoryaccount;
		DROP TABLE codewallet__core_vmmemoryallocatedmemory;
	`
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

func TestVmRamPostgresStore(t *testing.T) {
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
