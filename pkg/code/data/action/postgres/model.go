package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

const (
	tableName = "codewallet__core_action"
)

type model struct {
	Id                   sql.NullInt64  `db:"id"`
	Intent               string         `db:"intent"`
	IntentType           uint           `db:"intent_type"`
	ActionId             uint           `db:"action_id"`
	ActionType           uint           `db:"action_type"`
	Source               string         `db:"source"`
	Destination          sql.NullString `db:"destination"`
	Quantity             sql.NullInt64  `db:"quantity"`
	InitiatorPhoneNumber sql.NullString `db:"initiator_phone_number"`
	State                uint           `db:"state"`
	CreatedAt            time.Time      `db:"created_at"`
}

func toModel(obj *action.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	var destination sql.NullString
	if obj.Destination != nil {
		destination.Valid = true
		destination.String = *obj.Destination
	}

	var quantity sql.NullInt64
	if obj.Quantity != nil {
		quantity.Valid = true
		quantity.Int64 = int64(*obj.Quantity)
	}

	var initiatorPhoneNumber sql.NullString
	if obj.InitiatorPhoneNumber != nil {
		initiatorPhoneNumber.Valid = true
		initiatorPhoneNumber.String = *obj.InitiatorPhoneNumber
	}

	return &model{
		Intent:               obj.Intent,
		IntentType:           uint(obj.IntentType),
		ActionId:             uint(obj.ActionId),
		ActionType:           uint(obj.ActionType),
		Source:               obj.Source,
		Destination:          destination,
		Quantity:             quantity,
		InitiatorPhoneNumber: initiatorPhoneNumber,
		State:                uint(obj.State),
		CreatedAt:            obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *action.Record {
	var destination *string
	if obj.Destination.Valid {
		destination = &obj.Destination.String
	}

	var quantity *uint64
	if obj.Quantity.Valid {
		value := uint64(obj.Quantity.Int64)
		quantity = &value
	}

	var initiatorPhoneNumber *string
	if obj.InitiatorPhoneNumber.Valid {
		initiatorPhoneNumber = &obj.InitiatorPhoneNumber.String
	}

	return &action.Record{
		Id:                   uint64(obj.Id.Int64),
		Intent:               obj.Intent,
		IntentType:           intent.Type(obj.IntentType),
		ActionId:             uint32(obj.ActionId),
		ActionType:           action.Type(obj.ActionType),
		Source:               obj.Source,
		Destination:          destination,
		Quantity:             quantity,
		InitiatorPhoneNumber: initiatorPhoneNumber,
		State:                action.State(obj.State),
		CreatedAt:            obj.CreatedAt,
	}
}

func (m *model) dbUpdate(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		var quantityUpdateStmt string
		params := []interface{}{
			m.Intent,
			m.ActionId,
			m.State,
		}

		if m.ActionType == uint(action.CloseDormantAccount) {
			quantityUpdateStmt = ", quantity = $4"
			params = append(params, m.Quantity)
		}

		query := fmt.Sprintf(`UPDATE `+tableName+`
			SET state = $3%s
			WHERE intent = $1 AND action_id = $2
			RETURNING id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		`, quantityUpdateStmt)

		err := tx.QueryRowxContext(
			ctx,
			query,
			params...,
		).StructScan(m)
		if err != nil {
			return pgutil.CheckNoRows(err, action.ErrActionNotFound)
		}

		return nil
	})
}

func dbPutAllInTx(ctx context.Context, tx *sqlx.Tx, models []*model) ([]*model, error) {
	var res []*model

	query := `INSERT INTO ` + tableName + ` (intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at) VALUES `

	var parameters []interface{}
	for i, model := range models {
		if model.CreatedAt.IsZero() {
			model.CreatedAt = time.Now()
		}

		baseIndex := len(parameters)
		query += fmt.Sprintf(
			`($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)`,
			baseIndex+1, baseIndex+2, baseIndex+3, baseIndex+4, baseIndex+5, baseIndex+6, baseIndex+7, baseIndex+8, baseIndex+9, baseIndex+10,
		)

		if i != len(models)-1 {
			query += ","
		}

		parameters = append(
			parameters,
			model.Intent,
			model.IntentType,
			model.ActionId,
			model.ActionType,
			model.Source,
			model.Destination,
			model.Quantity,
			model.InitiatorPhoneNumber,
			model.State,
			model.CreatedAt,
		)
	}

	query += ` RETURNING id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at`

	err := tx.SelectContext(
		ctx,
		&res,
		query,
		parameters...,
	)
	if err != nil {
		return nil, pgutil.CheckUniqueViolation(err, action.ErrActionExists)
	}

	return res, nil
}

func dbGetById(ctx context.Context, db *sqlx.DB, intent string, actionId uint32) (*model, error) {
	res := &model{}

	query := `SELECT id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		FROM ` + tableName + `
		WHERE intent = $1 AND action_id = $2
		LIMIT 1`

	err := db.GetContext(ctx, res, query, intent, actionId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, action.ErrActionNotFound)
	}
	return res, nil
}

func dbGetAllByIntent(ctx context.Context, db *sqlx.DB, intent string) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		FROM ` + tableName + `
		WHERE intent = $1
		ORDER BY action_id ASC`

	err := db.SelectContext(ctx, &res, query, intent)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, action.ErrActionNotFound)
	}

	if len(res) == 0 {
		return nil, action.ErrActionNotFound
	}

	return res, nil
}

func dbGetAllByAddress(ctx context.Context, db *sqlx.DB, address string) ([]*model, error) {
	res := []*model{}

	query := `SELECT id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		FROM ` + tableName + `
		WHERE source = $1 OR destination = $1`

	err := db.SelectContext(ctx, &res, query, address)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, action.ErrActionNotFound)
	}

	if len(res) == 0 {
		return nil, action.ErrActionNotFound
	}

	return res, nil
}

func dbGetNetBalance(ctx context.Context, db *sqlx.DB, account string) (int64, error) {
	var res sql.NullInt64

	query := `SELECT
		(SELECT COALESCE(SUM(quantity), 0) FROM ` + tableName + ` WHERE destination = $1 AND state != $2) -
		(SELECT COALESCE(SUM(quantity), 0) FROM ` + tableName + ` WHERE source = $1 AND state != $2);`

	err := pgutil.ExecuteInTx(ctx, db, sql.LevelRepeatableRead, func(tx *sqlx.Tx) error {
		return tx.GetContext(
			ctx,
			&res,
			query,
			account,
			action.StateRevoked,
		)
	})
	if err != nil {
		return 0, err
	}

	if !res.Valid {
		return 0, nil
	}
	return res.Int64, nil
}
func dbGetNetBalanceBatch(ctx context.Context, db *sqlx.DB, accounts ...string) (map[string]int64, error) {
	if len(accounts) == 0 {
		return make(map[string]int64), nil
	}

	type Row struct {
		Account    string `db:"account"`
		NetBalance int64  `db:"net_balance"`
	}
	rows := []*Row{}

	individualFilters := make([]string, len(accounts))
	for i, vault := range accounts {
		individualFilters[i] = fmt.Sprintf("'%s'", vault)
	}
	accountFilter := strings.Join(individualFilters, ",")

	query := fmt.Sprintf(
		`(SELECT destination AS account, COALESCE(SUM(quantity), 0) AS net_balance FROM `+tableName+` WHERE destination IN (%s) AND state != $1 GROUP BY destination)
		UNION
		(SELECT source AS account, -COALESCE(SUM(quantity), 0) AS net_balance FROM `+tableName+` WHERE source IN (%s) AND state != $1 GROUP BY source);`,
		accountFilter, accountFilter,
	)
	err := pgutil.ExecuteInTx(ctx, db, sql.LevelRepeatableRead, func(tx *sqlx.Tx) error {
		return tx.SelectContext(
			ctx,
			&rows,
			query,
			action.StateRevoked,
		)
	})
	if err != nil {
		return nil, err
	}

	res := make(map[string]int64)
	for _, account := range accounts {
		res[account] = 0
	}
	for _, row := range rows {
		res[row.Account] += row.NetBalance
	}
	return res, nil
}

func dbGetGiftCardClaimedAction(ctx context.Context, db *sqlx.DB, giftCardVault string) (*model, error) {
	res := []*model{}

	query := `SELECT id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		FROM ` + tableName + `
		WHERE source = $1 AND action_type = $2 AND state != $3
		LIMIT 2`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		giftCardVault,
		action.NoPrivacyWithdraw,
		action.StateRevoked,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, action.ErrActionNotFound)
	}

	if len(res) == 0 {
		return nil, action.ErrActionNotFound
	} else if len(res) > 1 {
		return nil, action.ErrMultipleActionsFound
	}

	return res[0], nil
}

func dbGetGiftCardAutoReturnAction(ctx context.Context, db *sqlx.DB, giftCardVault string) (*model, error) {
	res := []*model{}

	query := `SELECT id, intent, intent_type, action_id, action_type, source, destination, quantity, initiator_phone_number, state, created_at
		FROM ` + tableName + `
		WHERE source = $1 AND action_type = $2 AND state != $3
		LIMIT 2`

	err := db.SelectContext(
		ctx,
		&res,
		query,
		giftCardVault,
		action.CloseDormantAccount,
		action.StateRevoked,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, action.ErrActionNotFound)
	}

	if len(res) == 0 {
		return nil, action.ErrActionNotFound
	} else if len(res) > 1 {
		return nil, action.ErrMultipleActionsFound
	}

	return res[0], nil
}
