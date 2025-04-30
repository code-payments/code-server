package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	fulfillmentTableName = "codewallet__core_fulfillment"
)

type fulfillmentModel struct {
	Id                       int64          `db:"id"`
	Intent                   string         `db:"intent"`
	IntentType               uint           `db:"intent_type"`
	ActionId                 uint           `db:"action_id"`
	ActionType               uint           `db:"action_type"`
	FulfillmentType          uint           `db:"fulfillment_type"`
	Data                     []byte         `db:"data"`
	Signature                sql.NullString `db:"signature"`
	Nonce                    sql.NullString `db:"nonce"`
	Blockhash                sql.NullString `db:"blockhash"`
	VirtualSignature         sql.NullString `db:"virtual_signature"`
	VirtualNonce             sql.NullString `db:"virtual_nonce"`
	VirtualBlockhash         sql.NullString `db:"virtual_blockhash"`
	Source                   string         `db:"source"`
	Destination              sql.NullString `db:"destination"`
	IntentOrderingIndex      uint64         `db:"intent_ordering_index"`
	ActionOrderingIndex      uint32         `db:"action_ordering_index"`
	FulfillmentOrderingIndex uint32         `db:"fulfillment_ordering_index"`
	DisableActiveScheduling  bool           `db:"disable_active_scheduling"`
	State                    uint           `db:"state"`
	CreatedAt                time.Time      `db:"created_at"`
	BatchInsertionId         int            `db:"batch_insertion_id"`
}

func toFulfillmentModel(obj *fulfillment.Record) (*fulfillmentModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now().UTC()
	}

	var signatureValue sql.NullString
	if obj.Signature != nil {
		signatureValue.Valid = true
		signatureValue.String = *obj.Signature
	}

	var nonceValue sql.NullString
	if obj.Nonce != nil {
		nonceValue.Valid = true
		nonceValue.String = *obj.Nonce
	}

	var blockHashValue sql.NullString
	if obj.Blockhash != nil {
		blockHashValue.Valid = true
		blockHashValue.String = *obj.Blockhash
	}

	var virtualSignatureValue sql.NullString
	if obj.VirtualSignature != nil {
		virtualSignatureValue.Valid = true
		virtualSignatureValue.String = *obj.VirtualSignature
	}

	var virtualNonceValue sql.NullString
	if obj.VirtualNonce != nil {
		virtualNonceValue.Valid = true
		virtualNonceValue.String = *obj.VirtualNonce
	}

	var virtualBlockHashValue sql.NullString
	if obj.VirtualBlockhash != nil {
		virtualBlockHashValue.Valid = true
		virtualBlockHashValue.String = *obj.VirtualBlockhash
	}

	var destinationValue sql.NullString
	if obj.Destination != nil {
		destinationValue.Valid = true
		destinationValue.String = *obj.Destination
	}

	return &fulfillmentModel{
		Id:                       int64(obj.Id),
		Intent:                   obj.Intent,
		IntentType:               uint(obj.IntentType),
		ActionId:                 uint(obj.ActionId),
		ActionType:               uint(obj.ActionType),
		FulfillmentType:          uint(obj.FulfillmentType),
		Data:                     obj.Data,
		Signature:                signatureValue,
		Nonce:                    nonceValue,
		Blockhash:                blockHashValue,
		VirtualSignature:         virtualSignatureValue,
		VirtualNonce:             virtualNonceValue,
		VirtualBlockhash:         virtualBlockHashValue,
		Source:                   obj.Source,
		Destination:              destinationValue,
		IntentOrderingIndex:      obj.IntentOrderingIndex,
		ActionOrderingIndex:      obj.ActionOrderingIndex,
		FulfillmentOrderingIndex: obj.FulfillmentOrderingIndex,
		DisableActiveScheduling:  obj.DisableActiveScheduling,
		State:                    uint(obj.State),
		CreatedAt:                obj.CreatedAt,
	}, nil
}

func fromFulfillmentModel(obj *fulfillmentModel) *fulfillment.Record {
	var sig *string
	if obj.Signature.Valid {
		sig = &obj.Signature.String
	}

	var nonce *string
	if obj.Nonce.Valid {
		nonce = &obj.Nonce.String
	}

	var blockhash *string
	if obj.Blockhash.Valid {
		blockhash = &obj.Blockhash.String
	}

	var virtualSig *string
	if obj.VirtualSignature.Valid {
		virtualSig = &obj.VirtualSignature.String
	}

	var virtualNonce *string
	if obj.VirtualNonce.Valid {
		virtualNonce = &obj.VirtualNonce.String
	}

	var virtualBlockhash *string
	if obj.VirtualBlockhash.Valid {
		virtualBlockhash = &obj.VirtualBlockhash.String
	}

	var destination *string
	if obj.Destination.Valid {
		destination = &obj.Destination.String
	}

	return &fulfillment.Record{
		Id:                       uint64(obj.Id),
		Intent:                   obj.Intent,
		IntentType:               intent.Type(obj.IntentType),
		ActionId:                 uint32(obj.ActionId),
		ActionType:               action.Type(obj.ActionType),
		FulfillmentType:          fulfillment.Type(obj.FulfillmentType),
		Data:                     obj.Data,
		Signature:                sig,
		Nonce:                    nonce,
		Blockhash:                blockhash,
		VirtualSignature:         virtualSig,
		VirtualNonce:             virtualNonce,
		VirtualBlockhash:         virtualBlockhash,
		Source:                   obj.Source,
		Destination:              destination,
		IntentOrderingIndex:      obj.IntentOrderingIndex,
		ActionOrderingIndex:      obj.ActionOrderingIndex,
		FulfillmentOrderingIndex: obj.FulfillmentOrderingIndex,
		DisableActiveScheduling:  obj.DisableActiveScheduling,
		State:                    fulfillment.State(obj.State),
		CreatedAt:                obj.CreatedAt.UTC(),
	}
}

func dbGetCount(ctx context.Context, db *sqlx.DB) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName
	err := db.GetContext(ctx, &res, query)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByState(ctx context.Context, db *sqlx.DB, state fulfillment.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE state = $1`
	err := db.GetContext(ctx, &res, query, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByStateGroupedByType(ctx context.Context, db *sqlx.DB, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	type countedType struct {
		Type  fulfillment.Type `db:"fulfillment_type"`
		Count uint64           `db:"count"`
	}

	var countedTypes []countedType
	query := `SELECT fulfillment_type, COUNT(*) as count FROM ` + fulfillmentTableName + `
		WHERE state = $1
		GROUP BY fulfillment_type
	`
	err := db.SelectContext(ctx, &countedTypes, query, state)
	if err != nil {
		return nil, err
	}

	res := make(map[fulfillment.Type]uint64)
	for _, countedType := range countedTypes {
		res[countedType.Type] = countedType.Count
	}
	return res, nil
}

func dbGetCountForMetrics(ctx context.Context, db *sqlx.DB, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	type countedType struct {
		Type  fulfillment.Type `db:"fulfillment_type"`
		Count uint64           `db:"count"`
	}

	// Simple optimization to avoid frequent large counts that aren't useful in
	// metrics tracking.
	exclusion := fulfillment.UnknownType
	if state == fulfillment.StateUnknown {
		exclusion = fulfillment.InitializeLockedTimelockAccount
	}

	var countedTypes []countedType
	query := `SELECT fulfillment_type, COUNT(*) as count FROM ` + fulfillmentTableName + `
		WHERE fulfillment_type != $1 AND state = $2
		GROUP BY fulfillment_type
	`
	err := db.SelectContext(ctx, &countedTypes, query, exclusion, state)
	if err != nil {
		return nil, err
	}

	res := make(map[fulfillment.Type]uint64)
	for _, countedType := range countedTypes {
		res[countedType.Type] = countedType.Count
	}
	return res, nil
}

func dbGetCountByStateAndAddress(ctx context.Context, db *sqlx.DB, state fulfillment.State, address string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE (state = $1 AND (source = $2 OR destination = $2))`
	err := db.GetContext(ctx, &res, query, state, address)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByTypeStateAndAddress(ctx context.Context, db *sqlx.DB, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE  ((source = $1 OR destination = $1) AND state = $2 AND fulfillment_type = $3)`
	err := db.GetContext(ctx, &res, query, address, state, fulfillmentType)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByTypeStateAndAddressAsSource(ctx context.Context, db *sqlx.DB, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE  (source = $1 AND state = $2 AND fulfillment_type = $3)`
	err := db.GetContext(ctx, &res, query, address, state, fulfillmentType)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByIntentAndState(ctx context.Context, db *sqlx.DB, intent string, state fulfillment.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE intent = $1 AND state = $2`
	err := db.GetContext(ctx, &res, query, intent, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByIntent(ctx context.Context, db *sqlx.DB, intent string) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + ` WHERE intent = $1`
	err := db.GetContext(ctx, &res, query, intent)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountByTypeActionAndState(ctx context.Context, db *sqlx.DB, intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + fulfillmentTableName + `
		WHERE intent = $1 AND action_id = $2 AND fulfillment_type = $3 AND state = $4`
	err := db.GetContext(ctx, &res, query, intentId, actionId, fulfillmentType, state)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetCountPendingByType(ctx context.Context, db *sqlx.DB) (map[fulfillment.Type]uint64, error) {
	type countedType struct {
		Type  fulfillment.Type `db:"fulfillment_type"`
		Count uint64           `db:"count"`
	}

	var countedTypes []countedType
	query := `SELECT fulfillment_type, COUNT(*) as count FROM ` + fulfillmentTableName + `
		WHERE state = $1
		GROUP BY fulfillment_type
	`
	err := db.SelectContext(ctx, &countedTypes, query, fulfillment.StatePending)
	if err != nil {
		return nil, err
	}

	res := make(map[fulfillment.Type]uint64)
	for _, countedType := range countedTypes {
		res[countedType.Type] = countedType.Count
	}
	return res, nil
}

func (m *fulfillmentModel) dbUpdate(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		if m.Id == 0 {
			return fulfillment.ErrFulfillmentNotFound
		}

		var preSortingUpdateStmt string
		params := []interface{}{
			m.Id,
			m.Signature,
			m.Nonce,
			m.Blockhash,
			m.Data,
			m.State,
			m.VirtualSignature,
			m.VirtualNonce,
			m.VirtualBlockhash,
		}

		if m.IntentType == uint(intent.SendPublicPayment) && m.FulfillmentType == uint(fulfillment.NoPrivacyWithdraw) {
			preSortingUpdateStmt = ", intent_ordering_index = $10, action_ordering_index = $11, fulfillment_ordering_index = $12"
			params = append(
				params,
				m.IntentOrderingIndex,
				m.ActionOrderingIndex,
				m.FulfillmentOrderingIndex,
			)
		}

		query := fmt.Sprintf(`UPDATE `+fulfillmentTableName+`
			SET signature = $2, nonce = $3, blockhash = $4, data = $5, state = $6, virtual_signature = $7, virtual_nonce = $8, virtual_blockhash = $9%s
			WHERE id = $1
			RETURNING
				id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at`,
			preSortingUpdateStmt,
		)

		err := tx.QueryRowxContext(
			ctx,
			query,
			params...,
		).StructScan(m)

		return pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	})
}

func dbPutAllInTx(ctx context.Context, tx *sqlx.Tx, models []*fulfillmentModel) ([]*fulfillmentModel, error) {
	var res []*fulfillmentModel

	query := `WITH inserted AS (`
	query += `INSERT INTO ` + fulfillmentTableName + ` (intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at, batch_insertion_id) VALUES `

	var parameters []interface{}
	for i, model := range models {
		if model.Id != 0 {
			return nil, fulfillment.ErrFulfillmentExists
		}

		if model.CreatedAt.IsZero() {
			model.CreatedAt = time.Now()
		}

		baseIndex := len(parameters)
		query += fmt.Sprintf(
			`($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)`,
			baseIndex+1, baseIndex+2, baseIndex+3, baseIndex+4, baseIndex+5, baseIndex+6, baseIndex+7, baseIndex+8, baseIndex+9, baseIndex+10, baseIndex+11, baseIndex+12, baseIndex+13, baseIndex+14, baseIndex+15, baseIndex+16, baseIndex+17, baseIndex+18, baseIndex+19, baseIndex+20, baseIndex+21,
		)

		if i != len(models)-1 {
			query += ","
		}

		batchInsertionId := i

		parameters = append(
			parameters,
			model.Intent,
			model.IntentType,
			model.ActionId,
			model.ActionType,
			model.FulfillmentType,
			model.Data,
			model.Signature,
			model.Nonce,
			model.Blockhash,
			model.VirtualSignature,
			model.VirtualNonce,
			model.VirtualBlockhash,
			model.Source,
			model.Destination,
			model.IntentOrderingIndex,
			model.ActionOrderingIndex,
			model.FulfillmentOrderingIndex,
			model.DisableActiveScheduling,
			model.State,
			model.CreatedAt,
			batchInsertionId,
		)
	}

	query += ` RETURNING id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at, batch_insertion_id) `

	// Kind of hacky, but we don't really have a great PK for on demand transactions
	// that allows us to update the corresponding record that was passed in (for example,
	// what was done for actions). Ideally we would've used the signature field, and
	// that won't exist in some cases until later.
	query += `SELECT * FROM inserted ORDER BY batch_insertion_id ASC`

	err := tx.SelectContext(
		ctx,
		&res,
		query,
		parameters...,
	)
	if err != nil {
		return nil, pgutil.CheckUniqueViolation(err, fulfillment.ErrFulfillmentExists)
	}

	return res, nil
}

func dbMarkAsActivelyScheduled(ctx context.Context, db *sqlx.DB, id uint64) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		if id == 0 {
			return fulfillment.ErrFulfillmentNotFound
		}

		query := `UPDATE ` + fulfillmentTableName + ` SET disable_active_scheduling = false WHERE id = $1`
		res, err := db.ExecContext(ctx, query, id)
		if err != nil {
			return err
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected == 0 {
			return fulfillment.ErrFulfillmentNotFound
		}
		return nil
	})
}

func dbGetById(ctx context.Context, db *sqlx.DB, id uint64) (*fulfillmentModel, error) {
	if id == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE id = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, id)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetBySignature(ctx context.Context, db *sqlx.DB, signature string) (*fulfillmentModel, error) {
	if len(signature) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE signature = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, signature)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetByVirtualSignature(ctx context.Context, db *sqlx.DB, signature string) (*fulfillmentModel, error) {
	if len(signature) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE virtual_signature = $1
		LIMIT 1`

	err := db.GetContext(ctx, res, query, signature)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetAllByState(ctx context.Context, db *sqlx.DB, state fulfillment.State, includeDisabledActiveScheduling bool, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*fulfillmentModel, error) {
	res := []*fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (state = $1 AND %s)
	`

	if includeDisabledActiveScheduling {
		query = fmt.Sprintf(query, "TRUE")
	} else {
		query = fmt.Sprintf(query, "disable_active_scheduling IS FALSE")
	}

	opts := []interface{}{state}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}

	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return res, nil
}

func dbGetAllByIntent(ctx context.Context, db *sqlx.DB, intent string, cursor q.Cursor, limit uint64, direction q.Ordering) ([]*fulfillmentModel, error) {
	res := []*fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (intent = $1)
	`

	opts := []interface{}{intent}
	query, opts = q.PaginateQuery(query, opts, cursor, limit, direction)

	err := db.SelectContext(ctx, &res, query, opts...)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}

	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return res, nil
}

func dbGetAllByAction(ctx context.Context, db *sqlx.DB, intentId string, actionId uint32) ([]*fulfillmentModel, error) {
	res := []*fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (intent = $1 and action_id = $2)
	`

	err := db.SelectContext(ctx, &res, query, intentId, actionId)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}

	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return res, nil
}

func dbGetAllByTypeAndAction(ctx context.Context, db *sqlx.DB, fulfillmentType fulfillment.Type, intentId string, actionId uint32) ([]*fulfillmentModel, error) {
	res := []*fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE intent = $1 AND action_id = $2 AND fulfillment_type = $3
	`

	err := db.SelectContext(ctx, &res, query, intentId, actionId, fulfillmentType)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}

	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return res, nil
}

func dbGetFirstSchedulableByAddressAsSource(ctx context.Context, db *sqlx.DB, address string) (*fulfillmentModel, error) {
	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (source = $1 AND (state = $2 OR state = $3))
		ORDER BY intent_ordering_index ASC, action_ordering_index ASC, fulfillment_ordering_index ASC
		LIMIT 1`

	err := db.GetContext(ctx, res, query, address, fulfillment.StateUnknown, fulfillment.StatePending)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetFirstSchedulableByAddressAsDestination(ctx context.Context, db *sqlx.DB, address string) (*fulfillmentModel, error) {
	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (destination = $1 AND (state = $2 OR state = $3))
		ORDER BY intent_ordering_index ASC, action_ordering_index ASC, fulfillment_ordering_index ASC
		LIMIT 1`

	err := db.GetContext(ctx, res, query, address, fulfillment.StateUnknown, fulfillment.StatePending)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetFirstSchedulableByType(ctx context.Context, db *sqlx.DB, fulfillmentType fulfillment.Type) (*fulfillmentModel, error) {
	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE (fulfillment_type = $1 AND (state = $2 OR state = $3))
		ORDER BY intent_ordering_index ASC, action_ordering_index ASC, fulfillment_ordering_index ASC
		LIMIT 1`

	err := db.GetContext(ctx, res, query, fulfillmentType, fulfillment.StateUnknown, fulfillment.StatePending)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}

func dbGetNextSchedulableByAddress(ctx context.Context, db *sqlx.DB, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*fulfillmentModel, error) {
	res := &fulfillmentModel{}

	query := `SELECT id, intent, intent_type, action_id, action_type, fulfillment_type, data, signature, nonce, blockhash, virtual_signature, virtual_nonce, virtual_blockhash, source, destination, intent_ordering_index, action_ordering_index, fulfillment_ordering_index, disable_active_scheduling, state, created_at
		FROM ` + fulfillmentTableName + `
		WHERE ((source = $1 OR destination = $1) AND (state = $2 OR state = $3) AND (intent_ordering_index > $4 OR (intent_ordering_index = $4 AND action_ordering_index > $5) OR (intent_ordering_index = $4 AND action_ordering_index = $5 AND fulfillment_ordering_index > $6)))
		ORDER BY intent_ordering_index ASC, action_ordering_index ASC, fulfillment_ordering_index ASC
		LIMIT 1`

	err := db.GetContext(
		ctx,
		res,
		query,
		address,
		fulfillment.StateUnknown,
		fulfillment.StatePending,
		intentOrderingIndex,
		actionOrderingIndex,
		fulfillmentOrderingIndex,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, fulfillment.ErrFulfillmentNotFound)
	}
	return res, nil
}
