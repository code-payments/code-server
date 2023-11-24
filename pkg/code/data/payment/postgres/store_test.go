package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/payment/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
		CREATE TABLE codewallet__core_payment (
			id serial NOT NULL PRIMARY KEY, 

			block_id bigint NULL, 
			block_time timestamp with time zone NULL, 
			transaction_id text NOT NULL, 
			transaction_index integer NOT NULL, 
			rendezvous_key text,
			is_external boolean NOT NULL default false, 

			source text NOT NULL, 
			destination text NOT NULL, 
			quantity bigint NOT NULL CHECK (quantity >= 0), 

			exchange_currency varchar(3) NOT NULL,
			region varchar(2),
			exchange_rate numeric(18, 9) NOT NULL,
			usd_market_value numeric(18, 9) NOT NULL,

			is_withdraw BOOL NOT NULL,

			confirmation_state integer NULL, 
			created_at timestamp with time zone NOT NULL, 

			CONSTRAINT codewallet__core_payment__uniq__tx_sig__and__index UNIQUE (transaction_id, transaction_index),
			CONSTRAINT codewallet__core_payment__currency_code CHECK (exchange_currency::text ~ '^[a-z]{3}$')
		);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_payment;
	`
)

var (
	testStore payment.Store
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

func TestPaymentPostgresStore(t *testing.T) {
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
