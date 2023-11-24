package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_appuser"
)

type model struct {
	ID          sql.NullInt64 `db:"id"`
	UserID      string        `db:"user_id"`
	PhoneNumber string        `db:"phone_number"`
	IsStaffuser bool          `db:"is_staff_user"`
	IsBanned    bool          `db:"is_banned"`
	CreatedAt   time.Time     `db:"created_at"`
}

func toModel(obj *user_identity.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	model := &model{
		UserID:      obj.ID.String(),
		PhoneNumber: *obj.View.PhoneNumber,
		IsStaffuser: obj.IsStaffUser,
		IsBanned:    obj.IsBanned,
		CreatedAt:   obj.CreatedAt.UTC(),
	}

	return model, nil
}

func fromModel(obj *model) (*user_identity.Record, error) {
	userID, err := user.GetUserIDFromString(obj.UserID)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing user id")
	}

	return &user_identity.Record{
		ID: userID,
		View: &user.View{
			PhoneNumber: &obj.PhoneNumber,
		},
		IsStaffUser: obj.IsStaffuser,
		IsBanned:    obj.IsBanned,
		CreatedAt:   obj.CreatedAt,
	}, nil
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(user_id, phone_number, is_staff_user, is_banned, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, phone_number, is_staff_user, is_banned, created_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.UserID,
		m.PhoneNumber,
		m.IsStaffuser,
		m.IsBanned,
		m.CreatedAt,
	).StructScan(m)
	return pgutil.CheckUniqueViolation(err, user_identity.ErrAlreadyExists)
}

func dbGetByID(ctx context.Context, db *sqlx.DB, id *user.UserID) (*model, error) {
	query := `SELECT id, user_id, phone_number, is_staff_user, is_banned, created_at FROM ` + tableName + `
		WHERE user_id = $1`

	result := &model{}
	err := db.GetContext(ctx, result, query, id.String())
	if err != nil {
		return nil, pgutil.CheckNoRows(err, user_identity.ErrNotFound)
	}
	return result, nil
}

func dbGetByView(ctx context.Context, db *sqlx.DB, view *user.View) (*model, error) {
	if err := view.Validate(); err != nil {
		return nil, err
	}

	query := `SELECT id, user_id, phone_number, is_staff_user, is_banned, created_at FROM ` + tableName + `
		WHERE phone_number = $1`

	result := &model{}
	err := db.GetContext(ctx, result, query, *view.PhoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, user_identity.ErrNotFound)
	}
	return result, nil
}
