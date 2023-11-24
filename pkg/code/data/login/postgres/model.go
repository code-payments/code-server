package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/login"
)

const (
	tableName = "codewallet__core_applogin"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	AppInstallId string `db:"app_install_id"`
	Owner        string `db:"owner"`

	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toModels(item *login.MultiRecord) ([]*model, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}

	var res []*model
	for _, owner := range item.Owners {
		res = append(res, &model{
			AppInstallId:  item.AppInstallId,
			Owner:         owner,
			LastUpdatedAt: item.LastUpdatedAt,
		})
	}
	return res, nil
}

func fromModel(m *model) *login.Record {
	return &login.Record{
		AppInstallId:  m.AppInstallId,
		Owner:         m.Owner,
		LastUpdatedAt: m.LastUpdatedAt,
	}
}

func fromModels(appInstallId string, models []*model) (*login.MultiRecord, error) {
	res := &login.MultiRecord{
		AppInstallId:  appInstallId,
		LastUpdatedAt: time.Now(),
	}

	var lastUpdatedAt time.Time
	for _, model := range models {
		if model.AppInstallId != appInstallId {
			return nil, errors.New("models are not from the expected app install")
		}

		res.Owners = append(res.Owners, model.Owner)

		if model.LastUpdatedAt.After(lastUpdatedAt) {
			lastUpdatedAt = model.LastUpdatedAt
		}
	}

	if len(models) > 0 {
		res.LastUpdatedAt = lastUpdatedAt
	}

	return res, nil
}

func (m *model) dbSaveInTx(ctx context.Context, tx *sqlx.Tx) error {
	m.LastUpdatedAt = time.Now()

	err := dbDeleteAllByInstallIdInTx(ctx, tx, m.AppInstallId)
	if err != nil {
		return err
	}

	err = dbDeleteAllByOwnerInTx(ctx, tx, m.Owner)
	if err != nil {
		return err
	}

	query := `INSERT INTO ` + tableName + `
		(app_install_id, owner, last_updated_at)
		VALUES ($1, $2, $3)
		RETURNING id, app_install_id, owner, last_updated_at`
	_, err = tx.ExecContext(
		ctx,
		query,
		m.AppInstallId,
		m.Owner,
		m.LastUpdatedAt,
	)
	return err
}

func dbDeleteAllByInstallIdInTx(ctx context.Context, tx *sqlx.Tx, appInstallId string) error {
	query := `DELETE FROM ` + tableName + `
			WHERE app_install_id = $1`
	_, err := tx.ExecContext(
		ctx,
		query,
		appInstallId,
	)
	return err
}

func dbDeleteAllByOwnerInTx(ctx context.Context, tx *sqlx.Tx, owner string) error {
	query := `DELETE FROM ` + tableName + `
			WHERE owner = $1`
	_, err := tx.ExecContext(
		ctx,
		query,
		owner,
	)
	return err
}

func dbGetAllByInstallId(ctx context.Context, db *sqlx.DB, appInstallId string) ([]*model, error) {
	var res []*model

	query := `SELECT id, app_install_id, owner, last_updated_at FROM ` + tableName + `
		WHERE app_install_id = $1`
	err := db.SelectContext(ctx, &res, query, appInstallId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, login.ErrLoginNotFound)
	}
	if len(res) == 0 {
		return nil, login.ErrLoginNotFound
	}
	return res, nil
}

func dbGetLatestByOwner(ctx context.Context, db *sqlx.DB, owner string) (*model, error) {
	var res model

	query := `SELECT id, app_install_id, owner, last_updated_at FROM ` + tableName + `
		WHERE owner = $1`
	err := db.GetContext(ctx, &res, query, owner)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, login.ErrLoginNotFound)
	}
	return &res, nil
}
