package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

const (
	tableName = "codewallet__core_webhook"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	WebhookId string `db:"webhook_id"`
	Url       string `db:"url"`
	Type      uint8  `db:"webhook_type"`

	Attempts uint8 `db:"attempts"`
	State    uint8 `db:"state"`

	CreatedAt     time.Time    `db:"created_at"`
	NextAttemptAt sql.NullTime `db:"next_attempt_at"`
}

func toModel(obj *webhook.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		WebhookId: obj.WebhookId,
		Url:       obj.Url,
		Type:      uint8(obj.Type),

		Attempts: obj.Attempts,
		State:    uint8(obj.State),

		CreatedAt: obj.CreatedAt,
		NextAttemptAt: sql.NullTime{
			Valid: obj.NextAttemptAt != nil,
			Time:  *pointer.TimeOrDefault(obj.NextAttemptAt, time.Time{}),
		},
	}, nil
}

func fromModel(obj *model) *webhook.Record {
	return &webhook.Record{
		Id: uint64(obj.Id.Int64),

		WebhookId: obj.WebhookId,
		Url:       obj.Url,
		Type:      webhook.Type(obj.Type),

		Attempts: obj.Attempts,
		State:    webhook.State(obj.State),

		CreatedAt:     obj.CreatedAt,
		NextAttemptAt: pointer.TimeIfValid(obj.NextAttemptAt.Valid, obj.NextAttemptAt.Time),
	}
}

func (m *model) dbPut(ctx context.Context, db *sqlx.DB) error {
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(webhook_id, url, webhook_type, attempts, state, created_at, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, webhook_id, url, webhook_type, attempts, state, created_at, next_attempt_at
		`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		return tx.QueryRowxContext(
			ctx,
			query,
			m.WebhookId,
			m.Url,
			m.Type,
			m.Attempts,
			m.State,
			m.CreatedAt,
			m.NextAttemptAt,
		).StructScan(m)
	})
	return pgutil.CheckUniqueViolation(err, webhook.ErrAlreadyExists)
}

func (m *model) dbUpdate(ctx context.Context, db *sqlx.DB) error {
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `UPDATE ` + tableName + `
			SET attempts = $2, state = $3, next_attempt_at = $4
			WHERE webhook_id = $1
			RETURNING id, webhook_id, url, webhook_type, attempts, state, created_at, next_attempt_at
		`

		return tx.QueryRowxContext(
			ctx,
			query,
			m.WebhookId,
			m.Attempts,
			m.State,
			m.NextAttemptAt,
		).StructScan(m)
	})
	return pgutil.CheckNoRows(err, webhook.ErrNotFound)
}

func dbGetByWebhookId(ctx context.Context, db *sqlx.DB, webhookId string) (*model, error) {
	var res model
	query := `SELECT id, webhook_id, url, webhook_type, attempts, state, created_at, next_attempt_at FROM ` + tableName + `
		WHERE webhook_id = $1
	`

	err := db.GetContext(ctx, &res, query, webhookId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, webhook.ErrNotFound)
	}
	return &res, nil
}

func dbCountByState(ctx context.Context, db *sqlx.DB, state webhook.State) (uint64, error) {
	var res uint64
	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE state = $1
	`

	err := db.GetContext(ctx, &res, query, state)
	if err != nil {
		return 0, err
	}
	return res, nil
}

func dbGetAllPendingReadyToSend(ctx context.Context, db *sqlx.DB, limit uint64) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, webhook_id, url, webhook_type, attempts, state, created_at, next_attempt_at FROM ` + tableName + `
		WHERE state = $1 AND next_attempt_at <= $2
		LIMIT $3
	`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		webhook.StatePending,
		time.Now(),
		limit,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, webhook.ErrNotFound)
	} else if len(res) == 0 {
		return nil, webhook.ErrNotFound
	}
	return res, nil
}
