package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/text/language"

	"github.com/code-payments/code-server/pkg/code/data/preferences"
	"github.com/code-payments/code-server/pkg/code/data/user"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_appuserpreferences"
)

type model struct {
	Id              sql.NullInt64 `db:"id"`
	DataContainerId string        `db:"data_container_id"`
	Locale          string        `db:"locale"`
	LastUpdatedAt   time.Time     `db:"last_updated_at"`
}

func toModel(obj *preferences.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		DataContainerId: obj.DataContainerId.String(),
		Locale:          obj.Locale.String(),
		LastUpdatedAt:   obj.LastUpdatedAt,
	}, nil
}

func fromModel(obj *model) (*preferences.Record, error) {
	dataContainerID, err := user.GetDataContainerIDFromString(obj.DataContainerId)
	if err != nil {
		return nil, err
	}

	locale, err := language.Parse(obj.Locale)
	if err != nil {
		return nil, err
	}

	return &preferences.Record{
		Id:              uint64(obj.Id.Int64),
		DataContainerId: *dataContainerID,
		Locale:          locale,
		LastUpdatedAt:   obj.LastUpdatedAt,
	}, nil
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(data_container_id, locale, last_updated_at)
		VALUES ($1, $2, $3)

		ON CONFLICT (data_container_id)
		DO UPDATE
			SET locale = $2, last_updated_at = $3
			WHERE ` + tableName + `.data_container_id = $1 


		RETURNING id, data_container_id, locale, last_updated_at
	`

	m.LastUpdatedAt = time.Now()

	return db.QueryRowxContext(
		ctx,
		query,
		m.DataContainerId,
		m.Locale,
		m.LastUpdatedAt.UTC(),
	).StructScan(m)
}

func dbGet(ctx context.Context, db *sqlx.DB, id *user.DataContainerID) (*model, error) {
	res := &model{}

	query := `SELECT id, data_container_id, locale, last_updated_at FROM ` + tableName + `
			WHERE data_container_id = $1`

	err := db.GetContext(
		ctx,
		res,
		query,
		id.String(),
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, preferences.ErrPreferencesNotFound)
	}

	return res, nil
}
