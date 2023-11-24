package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

const (
	tableName = "codewallet__core_pushtoken"
)

type model struct {
	Id              sql.NullInt64 `db:"id"`
	DataContainerId string        `db:"data_container_id"`
	PushToken       string        `db:"push_token"`
	TokenType       uint          `db:"token_type"`
	IsValid         bool          `db:"is_valid"`
	AppInstallId    string        `db:"app_install_id"` // Cannot be nullable, since it's a part of a unique constraint
	CreatedAt       time.Time     `db:"created_at"`
}

func toModel(obj *push.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		DataContainerId: obj.DataContainerId.String(),
		PushToken:       obj.PushToken,
		TokenType:       uint(obj.TokenType),
		IsValid:         obj.IsValid,
		AppInstallId:    *pointer.StringOrDefault(obj.AppInstallId, ""),
		CreatedAt:       obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) (*push.Record, error) {
	dataContainerID, err := user.GetDataContainerIDFromString(obj.DataContainerId)
	if err != nil {
		return nil, err
	}

	return &push.Record{
		Id:              uint64(obj.Id.Int64),
		DataContainerId: *dataContainerID,
		PushToken:       obj.PushToken,
		TokenType:       push.TokenType(obj.TokenType),
		IsValid:         obj.IsValid,
		AppInstallId:    pointer.StringIfValid(len(obj.AppInstallId) > 0, obj.AppInstallId),
		CreatedAt:       obj.CreatedAt,
	}, nil
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(data_container_id, push_token, token_type, is_valid, app_install_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, data_container_id, push_token, token_type, is_valid, app_install_id, created_at
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.DataContainerId,
		m.PushToken,
		m.TokenType,
		m.IsValid,
		m.AppInstallId,
		m.CreatedAt,
	).StructScan(m)

	return pgutil.CheckUniqueViolation(err, push.ErrTokenExists)
}

func dbMarkAsInvalid(ctx context.Context, db *sqlx.DB, pushToken string) error {
	query := `UPDATE ` + tableName + `
		SET is_valid = false
		WHERE push_token = $1
	`

	_, err := db.ExecContext(ctx, query, pushToken)
	return err
}

func dbDelete(ctx context.Context, db *sqlx.DB, pushToken string) error {
	query := `DELETE FROM ` + tableName + `
		WHERE push_token = $1
	`

	_, err := db.ExecContext(ctx, query, pushToken)
	return err
}

func dbGetAllValidByDataContainer(ctx context.Context, db *sqlx.DB, id *user.DataContainerID) ([]*model, error) {
	res := []*model{}

	query := `SELECT
		id, data_container_id, push_token, token_type, is_valid, app_install_id, created_at
		FROM ` + tableName + `
		WHERE data_container_id = $1 AND is_valid = true
	`

	err := db.SelectContext(ctx, &res, query, id.String())
	if err != nil {
		return nil, pgutil.CheckNoRows(err, push.ErrTokenNotFound)
	}

	if len(res) == 0 {
		return nil, push.ErrTokenNotFound
	}

	return res, nil
}
