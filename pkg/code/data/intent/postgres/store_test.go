package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/intent/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_paymentintent(
			id SERIAL NOT NULL PRIMARY KEY,

			intent_id TEXT NOT NULL UNIQUE,
			intent_type INTEGER NOT NULL,

			owner text NOT NULL,
			source text NULL,
			destination text NULL,
			destination_owner text NULL,

			quantity bigint NULL CHECK (quantity >= 0),

			treasury_pool text NULL,
			recent_root text NULL,

			exchange_currency varchar(3) NULL,
			exchange_rate numeric(18, 9) NULL,
			native_amount numeric(18, 9) NULL,
			usd_market_value numeric(18, 9) NULL,

			is_withdraw BOOL NOT NULL,
			is_deposit BOOL NOT NULL,
			is_remote_send BOOL NOT NULL,
			is_returned BOOL NOT NULL,
			is_issuer_voiding_gift_card BOOL NOT NULL,
			is_micro_payment BOOL NOT NULL,

			relationship_to TEXT NULL,

			phone_number text NULL,

			state integer NOT NULL,

			created_at timestamp with time zone NOT NULL
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_paymentintent;
	`
)

var (
	testStore intent.Store
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

func TestIntentPostgresStore(t *testing.T) {
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
