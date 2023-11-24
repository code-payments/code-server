package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"

	"github.com/code-payments/code-server/pkg/code/data/messaging"
)

const (
	tableName = "codewallet__core_message"
)

type model struct {
	Id        sql.NullInt64 `db:"id"`
	Account   string        `db:"account"`
	MessageID string        `db:"message_id"`
	Message   []byte        `db:"message"`
	CreatedAt time.Time     `db:"created_at"`
}

func toModel(record *messaging.Record) (*model, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}

	if len(record.Account) == 0 {
		return nil, errors.New("empty account")
	}

	if record.Message == nil || len(record.Message) == 0 {
		return nil, errors.New("empty message id")
	}

	return &model{
		Account:   record.Account,
		MessageID: record.MessageID.String(),
		Message:   record.Message,
		// The only time we call toModel is on create, so it's fine to default
		// to UTC now.
		CreatedAt: time.Now().UTC(),
	}, nil
}

func fromModel(obj *model) (*messaging.Record, error) {
	parsedMessageID, err := uuid.Parse(obj.MessageID)
	if err != nil {
		return nil, errors.Wrap(err, "failure parsing message id")
	}

	return &messaging.Record{
		Account:   obj.Account,
		MessageID: parsedMessageID,
		Message:   obj.Message,
	}, nil
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
		(
			account, message_id, message, created_at
		) VALUES ($1,$2,$3,$4) RETURNING *;`

		err := tx.QueryRowxContext(ctx, query,
			m.Account,
			m.MessageID,
			m.Message,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, messaging.ErrDuplicateMessageID)
	})
}

func dbGetAllForAccount(ctx context.Context, db *sqlx.DB, account string) ([]*model, error) {
	res := []*model{}

	query := `SELECT account, message_id, message FROM ` + tableName + `
		WHERE account = $1`

	err := db.SelectContext(ctx, &res, query, account)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func dbDelete(ctx context.Context, db *sqlx.DB, account, messageID string) error {
	query := `DELETE FROM ` + tableName + `
		WHERE account = $1 AND message_id = $2;`
	_, err := db.ExecContext(ctx, query, account, messageID)
	return err
}
