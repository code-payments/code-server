package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/deposit"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_externaldeposit"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Signature      string  `db:"signature"`
	Destination    string  `db:"destination"`
	Amount         uint64  `db:"amount"`
	UsdMarketValue float64 `db:"usd_market_value"`

	Slot              uint64 `db:"slot"`
	ConfirmationState int    `db:"confirmation_state"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *deposit.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Signature:      obj.Signature,
		Destination:    obj.Destination,
		Amount:         obj.Amount,
		UsdMarketValue: obj.UsdMarketValue,

		Slot:              obj.Slot,
		ConfirmationState: int(obj.ConfirmationState),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *deposit.Record {
	return &deposit.Record{
		Id: uint64(obj.Id.Int64),

		Signature:      obj.Signature,
		Destination:    obj.Destination,
		Amount:         obj.Amount,
		UsdMarketValue: obj.UsdMarketValue,

		Slot:              obj.Slot,
		ConfirmationState: transaction.Confirmation(obj.ConfirmationState),

		CreatedAt: obj.CreatedAt,
	}
}

func (m *model) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(signature, destination, amount, usd_market_value, slot, confirmation_state, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)

		ON CONFLICT(signature, destination)
		DO UPDATE
			SET slot = $5, confirmation_state = $6
			WHERE ` + tableName + `.signature = $1 AND ` + tableName + `.destination = $2

		RETURNING id, signature, destination, amount, usd_market_value, slot, confirmation_state, created_at`

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	return db.QueryRowxContext(
		ctx,
		query,
		m.Signature,
		m.Destination,
		m.Amount,
		m.UsdMarketValue,
		m.Slot,
		m.ConfirmationState,
		m.CreatedAt,
	).StructScan(m)
}

func dbGet(ctx context.Context, db *sqlx.DB, signature, account string) (*model, error) {
	var res model

	query := `SELECT id, signature, destination, amount, usd_market_value, slot, confirmation_state, created_at FROM ` + tableName + `
		WHERE signature = $1 AND destination = $2
	`

	err := db.GetContext(ctx, &res, query, signature, account)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, deposit.ErrDepositNotFound)
	}
	return &res, nil
}

func dbGetQuarkAmount(ctx context.Context, db *sqlx.DB, account string) (uint64, error) {
	var res sql.NullInt64

	query := `SELECT SUM(amount) FROM ` + tableName + `
		WHERE destination = $1 AND confirmation_state = $2
	`

	err := pgutil.ExecuteInTx(ctx, db, sql.LevelRepeatableRead, func(tx *sqlx.Tx) error {
		return db.GetContext(ctx, &res, query, account, transaction.ConfirmationFinalized)
	})
	if err != nil {
		return 0, err
	}

	if !res.Valid {
		return 0, nil
	}
	return uint64(res.Int64), nil
}

func dbGetQuarkAmountBatch(ctx context.Context, db *sqlx.DB, accounts ...string) (map[string]uint64, error) {
	if len(accounts) == 0 {
		return make(map[string]uint64), nil
	}

	type Row struct {
		Destination string `db:"destination"`
		Amount      int64  `db:"amount"`
	}
	rows := []*Row{}

	individualFilters := make([]string, len(accounts))
	for i, vault := range accounts {
		individualFilters[i] = fmt.Sprintf("'%s'", vault)
	}
	accountFilter := strings.Join(individualFilters, ",")

	query := fmt.Sprintf(`SELECT destination, SUM(amount) AS amount FROM `+tableName+`
		WHERE destination IN (%s) AND confirmation_state = $1
		GROUP BY destination
	`, accountFilter)

	err := pgutil.ExecuteInTx(ctx, db, sql.LevelRepeatableRead, func(tx *sqlx.Tx) error {
		return tx.SelectContext(ctx, &rows, query, transaction.ConfirmationFinalized)
	})
	if err != nil {
		return nil, err
	}

	res := make(map[string]uint64)
	for _, account := range accounts {
		res[account] = 0
	}
	for _, row := range rows {
		res[row.Destination] = uint64(row.Amount)
	}
	return res, nil
}

func dbGetUsdAmount(ctx context.Context, db *sqlx.DB, account string) (float64, error) {
	var res sql.NullFloat64

	query := `SELECT SUM(usd_market_value) FROM ` + tableName + `
		WHERE destination = $1 AND confirmation_state = $2
	`

	err := pgutil.ExecuteInTx(ctx, db, sql.LevelRepeatableRead, func(tx *sqlx.Tx) error {
		return db.GetContext(ctx, &res, query, account, transaction.ConfirmationFinalized)
	})
	if err != nil {
		return 0, err
	}

	if !res.Valid {
		return 0, nil
	}
	return res.Float64, nil
}
