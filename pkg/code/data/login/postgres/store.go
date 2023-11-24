package postgres

import (
	"context"
	"database/sql"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/login"
)

type store struct {
	db *sqlx.DB
}

// New returns a new in psotres login.Store
func New(db *sql.DB) login.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Save implements login.Store.Save
//
// todo: An awkward implementation, but enables a quick migration to multi-device
// logins if that every becomes a thing.
func (s *store) Save(ctx context.Context, record *login.MultiRecord) error {
	models, err := toModels(record)
	if err != nil {
		return err
	}

	var newRecord *login.MultiRecord
	err = pgutil.ExecuteInTx(ctx, s.db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		for _, model := range models {
			err := model.dbSaveInTx(ctx, tx)
			if err != nil {
				return err
			}
		}

		if len(models) == 0 {
			err = dbDeleteAllByInstallIdInTx(ctx, tx, record.AppInstallId)
			if err != nil {
				return err
			}
		}

		newRecord, err = fromModels(record.AppInstallId, models)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	newRecord.CopyTo(record)
	return nil
}

// GetAllByInstallId implements login.Store.GetAllByInstallId
func (s *store) GetAllByInstallId(ctx context.Context, appInstallId string) (*login.MultiRecord, error) {
	models, err := dbGetAllByInstallId(ctx, s.db, appInstallId)
	if err != nil {
		return nil, err
	}

	return fromModels(appInstallId, models)
}

// GetLatestByOwner implements login.Store.GetLatestByOwner
func (s *store) GetLatestByOwner(ctx context.Context, owner string) (*login.Record, error) {
	model, err := dbGetLatestByOwner(ctx, s.db, owner)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}
