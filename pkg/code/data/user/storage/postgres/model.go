package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_storage "github.com/code-payments/code-server/pkg/code/data/user/storage"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_appuserstorage"
)

type model struct {
	ID           sql.NullInt64 `db:"id"`
	ContainerID  string        `db:"container_id"`
	OwnerAccount string        `db:"owner_account"`
	PhoneNumber  string        `db:"phone_number"`
	CreatedAt    time.Time     `db:"created_at"`
}

func toModel(obj *user_storage.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		ContainerID:  obj.ID.String(),
		OwnerAccount: obj.OwnerAccount,
		PhoneNumber:  *obj.IdentifyingFeatures.PhoneNumber,
		CreatedAt:    obj.CreatedAt.UTC(),
	}, nil
}

func fromModel(obj *model) (*user_storage.Record, error) {
	dataContainerID, err := user.GetDataContainerIDFromString(obj.ContainerID)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing data container id")
	}

	return &user_storage.Record{
		ID:           dataContainerID,
		OwnerAccount: obj.OwnerAccount,
		IdentifyingFeatures: &user.IdentifyingFeatures{
			PhoneNumber: &obj.PhoneNumber,
		},
		CreatedAt: obj.CreatedAt.UTC(),
	}, nil
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(container_id, owner_account, phone_number, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, container_id, owner_account, phone_number, created_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.ContainerID,
		m.OwnerAccount,
		m.PhoneNumber,
		m.CreatedAt.UTC(),
	).StructScan(m)
	return pgutil.CheckUniqueViolation(err, user_storage.ErrAlreadyExists)
}

func dbGetByID(ctx context.Context, db *sqlx.DB, id *user.DataContainerID) (*model, error) {
	query := `SELECT id, container_id, owner_account, phone_number, created_at FROM ` + tableName + `
		WHERE container_id = $1`

	result := &model{}
	err := db.GetContext(ctx, result, query, id.String())
	if err != nil {
		return nil, pgutil.CheckNoRows(err, user_storage.ErrNotFound)
	}
	return result, nil
}

func dbGetByFeatures(ctx context.Context, db *sqlx.DB, ownerAccount string, features *user.IdentifyingFeatures) (*model, error) {
	if err := features.Validate(); err != nil {
		return nil, err
	}

	query := `SELECT id, container_id, owner_account, phone_number, created_at FROM ` + tableName + `
		WHERE owner_account = $1 AND phone_number = $2`

	result := &model{}
	err := db.GetContext(ctx, result, query, ownerAccount, *features.PhoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, user_storage.ErrNotFound)
	}
	return result, nil
}
