package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_paymentrequest(
			id SERIAL NOT NULL PRIMARY KEY,

			intent TEXT NOT NULL UNIQUE,

			destination_token_account TEXT NULL,
			exchange_currency VARCHAR(3) NULL,
			native_amount NUMERIC(18, 9) NULL,
			exchange_rate NUMERIC(18, 9) NULL,
			quantity BIGINT NULL CHECK (quantity >= 0),

			domain TEXT NULL,
			is_verified BOOL NOT NULL,

			created_at TIMESTAMP WITH TIME ZONE NOT NULL
		);

		CREATE TABLE codewallet__core_paymentrequestfees(
			id SERIAL NOT NULL PRIMARY KEY,

			intent TEXT NOT NULL,

			destination_token_account TEXT NULL,
			bps INTEGER NOT NULL,

			CONSTRAINT codewallet__core_paymentrequestfees__uniq__intent__and__destination_token_account UNIQUE (intent, destination_token_account)
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_paymentrequest;
		DROP TABLE codewallet__core_paymentrequestfees;
	`
)

var (
	testStore paymentrequest.Store
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

func TestPaymentRequestPostgresStore(t *testing.T) {
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
