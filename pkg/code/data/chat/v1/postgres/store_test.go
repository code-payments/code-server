package postgres

import (
	"database/sql"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/sirupsen/logrus"

	chat "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/chat/v1/tests"

	postgrestest "github.com/code-payments/code-server/pkg/database/postgres/test"

	_ "github.com/jackc/pgx/v4/stdlib"
)

var (
	testStore chat.Store
	teardown  func()
)

const (
	// Used for testing ONLY, the table and migrations are external to this repository
	tableCreate = `
	CREATE TABLE codewallet__core_chat (
		id SERIAL NOT NULL PRIMARY KEY,

		chat_id BYTEA NOT NULL,
		chat_type INTEGER NOT NULL,
		is_verified BOOL NOT NULL,

		member1 TEXT NOT NULL,
		member2 TEXT NOT NULL,

		read_pointer TEXT NULL,

		is_muted BOOL NOT NULL,
		is_unsubscribed BOOL NOT NULL,

		created_at TIMESTAMP WITH TIME ZONE NOT NULL,

		CONSTRAINT codewallet__core_chat__uniq__chat_id UNIQUE (chat_id)
	);

	CREATE TABLE codewallet__core_chatmessage (
		id SERIAL NOT NULL PRIMARY KEY,

		chat_id BYTEA NOT NULL,

		message_id TEXT NOT NULL,
		data BYTEA NOT NULL,

		is_silent BOOL NOT NULL,
		content_length INTEGER NOT NULL,

		timestamp TIMESTAMP WITH TIME ZONE NOT NULL,

		CONSTRAINT codewallet__core_chatmessage__uniq__chat_id__and__message_id UNIQUE (chat_id, message_id)
	);
	`

	// Used for testing ONLY, the table and migrations are external to this repository
	tableDestroy = `
		DROP TABLE codewallet__core_chat;
		DROP TABLE codewallet__core_chatmessage;
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

func TestChatPostgresStore(t *testing.T) {
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
