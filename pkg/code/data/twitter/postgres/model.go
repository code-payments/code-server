package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-protobuf-api/generated/go/user/v1"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_twitteruser"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Username      string `db:"username"`
	Name          string `db:"name"`
	ProfilePicUrl string `db:"profile_pic_url"`
	VerifiedType  uint8  `db:"verified_type"`
	FollowerCount uint32 `db:"follower_count"`

	TipAddress string `db:"tip_address"`

	CreatedAt     time.Time `db:"created_at"`
	LastUpdatedAt time.Time `db:"last_updated_at"`
}

func toModel(r *twitter.Record) (*model, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Username:      r.Username,
		Name:          r.Name,
		ProfilePicUrl: r.ProfilePicUrl,
		VerifiedType:  uint8(r.VerifiedType),
		FollowerCount: r.FollowerCount,

		TipAddress: r.TipAddress,

		CreatedAt:     r.CreatedAt,
		LastUpdatedAt: r.LastUpdatedAt,
	}, nil
}

func fromModel(m *model) *twitter.Record {
	return &twitter.Record{
		Id: uint64(m.Id.Int64),

		Username:      m.Username,
		Name:          m.Name,
		ProfilePicUrl: m.ProfilePicUrl,
		VerifiedType:  user.GetTwitterUserResponse_VerifiedType(m.VerifiedType),
		FollowerCount: m.FollowerCount,

		TipAddress: m.TipAddress,

		CreatedAt:     m.CreatedAt,
		LastUpdatedAt: m.LastUpdatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		query := `INSERT INTO ` + tableName + `
			(username, name, profile_pic_url, verified_type, follower_count, tip_address, created_at, last_updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)

			ON CONFLICT (username)
			DO UPDATE
				SET name = $2, profile_pic_url = $3, verified_type = $4, follower_count = $5, tip_address = $6, created_at = $7, last_updated_at = $8
				WHERE ` + tableName + `.username = $1 

			RETURNING
			id, username, name, profile_pic_url, verified_type, follower_count, tip_address, created_at, last_updated_at`

		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}
		m.LastUpdatedAt = time.Now()

		err := tx.QueryRowxContext(
			ctx,
			query,
			m.Username,
			m.Name,
			m.ProfilePicUrl,
			m.VerifiedType,
			m.FollowerCount,
			m.TipAddress,
			m.CreatedAt,
			m.LastUpdatedAt,
		).StructScan(m)

		return pgutil.CheckNoRows(err, intent.ErrInvalidIntent)
	})
}

func dbGet(ctx context.Context, db *sqlx.DB, username string) (*model, error) {
	res := &model{}

	query := `SELECT
		id, username, name, profile_pic_url, verified_type, follower_count, tip_address, created_at, last_updated_at
		FROM ` + tableName + `
		WHERE username = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, username)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, twitter.ErrUserNotFound)
	}
	return res, nil
}
