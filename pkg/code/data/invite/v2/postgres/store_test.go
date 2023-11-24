package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
	"github.com/code-payments/code-server/pkg/code/data/invite/v2/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var (
	testStore invite.Store
	teardown  func()
)

type Schema struct {
	create string
	drop   string
}

var defaultSchema = Schema{
	// Used for testing ONLY, the table and migrations are external to this repository
	create: `
		CREATE TABLE codewallet__core_inviteuserv2(
			id SERIAL NOT NULL PRIMARY KEY,

			phone_number TEXT NOT NULL,
			invited_by TEXT,
			invited TIMESTAMP WITH TIME ZONE,
			invite_count INTEGER NOT NULL,
			invites_sent INTEGER NOT NULL,
			deposit_invites_received BOOL NOT NULL,
			is_revoked BOOL NOT NULL,

			CONSTRAINT codewallet__core_inviteuserv2__uniq__phone_number UNIQUE (phone_number)
		);

		CREATE TABLE codewallet__core_influencercode(
			code TEXT NOT NULL PRIMARY KEY,
			invite_count INTEGER NOT NULL,
			invites_sent INTEGER NOT NULL,
			is_revoked BOOL NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE
		);
	`,
	// Used for testing ONLY, the table and migrations are external to this repository
	drop: `
		DROP TABLE codewallet__core_inviteuserv2;
		DROP TABLE codewallet__core_influencercode;
	`,
}

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

func TestInviteV2PostgresStore(t *testing.T) {
	tests.RunTests(t, testStore, teardown)
}

func createTestTables(db *sql.DB) error {
	_, err := db.Exec(defaultSchema.create)
	if err != nil {
		logrus.StandardLogger().WithError(err).Error("could not create test tables")
		return err
	}
	return nil
}

func resetTestTables(db *sql.DB) error {
	_, err := db.Exec(defaultSchema.drop)
	if err != nil {
		logrus.StandardLogger().WithError(err).Error("could not drop test tables")
		return err
	}

	return createTestTables(db)
}
