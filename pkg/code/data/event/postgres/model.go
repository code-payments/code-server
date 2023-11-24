package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/event"
)

const (
	tableName = "codewallet__core_event"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	EventId   string `db:"event_id"`
	EventType uint32 `db:"event_type"`

	SourceCodeAccount      string         `db:"source_code_account"`
	DestinationCodeAccount sql.NullString `db:"destination_code_account"`
	ExternalTokenAccount   sql.NullString `db:"external_token_account"`

	SourceIdentity      string         `db:"source_identity"`
	DestinationIdentity sql.NullString `db:"destination_identity"`

	SourceClientIp           sql.NullString `db:"source_client_ip"`
	SourceClientCity         sql.NullString `db:"source_client_city"`
	SourceClientCountry      sql.NullString `db:"source_client_country"`
	DestinationClientIp      sql.NullString `db:"destination_client_ip"`
	DestinationClientCity    sql.NullString `db:"destination_client_city"`
	DestinationClientCountry sql.NullString `db:"destination_client_country"`

	UsdValue sql.NullFloat64 `db:"usd_value"`

	SpamConfidence float64 `db:"spam_confidence"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *event.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		EventId:   obj.EventId,
		EventType: uint32(obj.EventType),

		SourceCodeAccount: obj.SourceCodeAccount,
		DestinationCodeAccount: sql.NullString{
			Valid:  obj.DestinationCodeAccount != nil,
			String: *pointer.StringOrDefault(obj.DestinationCodeAccount, ""),
		},
		ExternalTokenAccount: sql.NullString{
			Valid:  obj.ExternalTokenAccount != nil,
			String: *pointer.StringOrDefault(obj.ExternalTokenAccount, ""),
		},

		SourceIdentity: obj.SourceIdentity,
		DestinationIdentity: sql.NullString{
			Valid:  obj.DestinationIdentity != nil,
			String: *pointer.StringOrDefault(obj.DestinationIdentity, ""),
		},

		SourceClientIp: sql.NullString{
			Valid:  obj.SourceClientIp != nil,
			String: *pointer.StringOrDefault(obj.SourceClientIp, ""),
		},
		SourceClientCity: sql.NullString{
			Valid:  obj.SourceClientCity != nil,
			String: *pointer.StringOrDefault(obj.SourceClientCity, ""),
		},
		SourceClientCountry: sql.NullString{
			Valid:  obj.SourceClientCountry != nil,
			String: *pointer.StringOrDefault(obj.SourceClientCountry, ""),
		},
		DestinationClientIp: sql.NullString{
			Valid:  obj.DestinationClientIp != nil,
			String: *pointer.StringOrDefault(obj.DestinationClientIp, ""),
		},
		DestinationClientCity: sql.NullString{
			Valid:  obj.DestinationClientCity != nil,
			String: *pointer.StringOrDefault(obj.DestinationClientCity, ""),
		},
		DestinationClientCountry: sql.NullString{
			Valid:  obj.DestinationClientCountry != nil,
			String: *pointer.StringOrDefault(obj.DestinationClientCountry, ""),
		},

		UsdValue: sql.NullFloat64{
			Valid:   obj.UsdValue != nil,
			Float64: *pointer.Float64OrDefault(obj.UsdValue, 0),
		},

		SpamConfidence: obj.SpamConfidence,

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *event.Record {
	return &event.Record{
		Id: uint64(obj.Id.Int64),

		EventId:   obj.EventId,
		EventType: event.EventType(obj.EventType),

		SourceCodeAccount:      obj.SourceCodeAccount,
		DestinationCodeAccount: pointer.StringIfValid(obj.DestinationCodeAccount.Valid, obj.DestinationCodeAccount.String),
		ExternalTokenAccount:   pointer.StringIfValid(obj.ExternalTokenAccount.Valid, obj.ExternalTokenAccount.String),

		SourceIdentity:      obj.SourceIdentity,
		DestinationIdentity: pointer.StringIfValid(obj.DestinationIdentity.Valid, obj.DestinationIdentity.String),

		SourceClientIp:           pointer.StringIfValid(obj.SourceClientIp.Valid, obj.SourceClientIp.String),
		SourceClientCity:         pointer.StringIfValid(obj.SourceClientCity.Valid, obj.SourceClientCity.String),
		SourceClientCountry:      pointer.StringIfValid(obj.SourceClientCountry.Valid, obj.SourceClientCountry.String),
		DestinationClientIp:      pointer.StringIfValid(obj.DestinationClientIp.Valid, obj.DestinationClientIp.String),
		DestinationClientCity:    pointer.StringIfValid(obj.DestinationClientCity.Valid, obj.DestinationClientCity.String),
		DestinationClientCountry: pointer.StringIfValid(obj.DestinationClientCountry.Valid, obj.DestinationClientCountry.String),

		UsdValue: pointer.Float64IfValid(obj.UsdValue.Valid, obj.UsdValue.Float64),

		SpamConfidence: obj.SpamConfidence,

		CreatedAt: obj.CreatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(event_id, event_type, source_code_account, destination_code_account, external_token_account, source_identity, destination_identity, source_client_ip, source_client_city, source_client_country, destination_client_ip, destination_client_city, destination_client_country, usd_value, spam_confidence, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (event_id)
			DO UPDATE
				SET destination_code_account = $4, destination_identity = $7, destination_client_ip = $11, destination_client_city = $12, destination_client_country = $13, spam_confidence = $15
				WHERE ` + tableName + `.event_id = $1
			RETURNING id, event_id, event_type, source_code_account, destination_code_account, external_token_account, source_identity, destination_identity, source_client_ip, source_client_city, source_client_country, destination_client_ip, destination_client_city, destination_client_country, usd_value, spam_confidence, created_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		return db.QueryRowxContext(
			ctx,
			query,

			m.EventId,
			m.EventType,

			m.SourceCodeAccount,
			m.DestinationCodeAccount,
			m.ExternalTokenAccount,

			m.SourceIdentity,
			m.DestinationIdentity,

			m.SourceClientIp,
			m.SourceClientCity,
			m.SourceClientCountry,
			m.DestinationClientIp,
			m.DestinationClientCity,
			m.DestinationClientCountry,

			m.UsdValue,

			m.SpamConfidence,

			m.CreatedAt,
		).StructScan(m)
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, id string) (*model, error) {
	var res model

	query := `SELECT id, event_id, event_type, source_code_account, destination_code_account, external_token_account, source_identity, destination_identity, source_client_ip, source_client_city, source_client_country, destination_client_ip, destination_client_city, destination_client_country, usd_value, spam_confidence, created_at FROM ` + tableName + `
		WHERE event_id = $1
	`

	err := db.GetContext(ctx, &res, query, id)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, event.ErrEventNotFound)
	}
	return &res, nil
}
