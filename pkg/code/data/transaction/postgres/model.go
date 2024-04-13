package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/transaction"
	pg "github.com/code-payments/code-server/pkg/database/postgres"
	q "github.com/code-payments/code-server/pkg/database/query"
)

const (
	tableNameTx        = "codewallet__core_transaction"
	tableNameTxBalance = "codewallet__core_transactiontokenbalance"
)

type transactionModel struct {
	Id                sql.NullInt64 `db:"id"`
	Signature         string        `db:"signature"`
	Slot              uint64        `db:"block_id"`
	BlockTime         time.Time     `db:"block_time"`
	Data              []byte        `db:"raw_data"`
	Fee               sql.NullInt64 `db:"fee"`
	HasErrors         bool          `db:"has_errors"`
	ConfirmationState uint          `db:"confirmation_state"`
	Confirmations     uint          `db:"confirmations"`
	CreatedAt         time.Time     `db:"created_at"`
}

func toTxModel(obj *transaction.Record) *transactionModel {
	var fee sql.NullInt64
	if obj.Fee != nil {
		fee = sql.NullInt64{Valid: obj.Fee != nil, Int64: int64(*obj.Fee)}
	}

	if obj.CreatedAt.IsZero() {
		obj.CreatedAt = time.Now()
	}

	return &transactionModel{
		Id:                sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		Signature:         obj.Signature,
		Slot:              obj.Slot,
		BlockTime:         obj.BlockTime.UTC(),
		Data:              obj.Data,
		Fee:               fee,
		HasErrors:         obj.HasErrors,
		ConfirmationState: uint(obj.ConfirmationState),
		Confirmations:     uint(obj.Confirmations),
		CreatedAt:         obj.CreatedAt.UTC(),
	}
}

func fromTxModel(obj *transactionModel) *transaction.Record {
	var fee uint64
	if obj.Fee.Valid {
		fee = uint64(obj.Fee.Int64)
	}

	return &transaction.Record{
		Id:                uint64(obj.Id.Int64),
		Signature:         obj.Signature,
		Slot:              obj.Slot,
		BlockTime:         obj.BlockTime.UTC(),
		Data:              obj.Data,
		Fee:               &fee,
		HasErrors:         obj.HasErrors,
		ConfirmationState: transaction.Confirmation(obj.ConfirmationState),
		Confirmations:     uint64(obj.Confirmations),
		CreatedAt:         obj.CreatedAt.UTC(),
	}
}

func makeAllQuery(table, columns, condition string, ordering q.Ordering) string {
	var query string

	query = "SELECT " + columns + " FROM " + table + " WHERE"

	order := q.FromOrderingWithFallback(ordering, "ASC")

	query = query + " (" + condition + ") "
	query = query + " ORDER BY block_id " + order + ", id " + order
	query = query + " LIMIT $1"

	//fmt.Printf("Query: %s\n", query)

	return query
}

func (self *transactionModel) txSaveTx(ctx context.Context, tx *sqlx.Tx) error {
	query := `INSERT INTO ` + tableNameTx + `
		(
			signature, block_id, block_time, raw_data, fee, has_errors,
			confirmation_state, confirmations, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (signature) DO UPDATE
		SET
			block_id 			= $2,
			block_time 			= $3,
			fee 				= $5,
			has_errors 			= $6,
			confirmation_state 	= $7,
			confirmations 		= $8
			WHERE ` + tableNameTx + `.signature = $1
		RETURNING *;`

	err := tx.QueryRowxContext(ctx, query,
		self.Signature,
		self.Slot,
		self.BlockTime,
		self.Data,
		self.Fee,
		self.HasErrors,
		self.ConfirmationState,
		self.Confirmations,
		self.CreatedAt,
	).StructScan(self)

	return err
}

func dbGetTx(ctx context.Context, db *sqlx.DB, sig string) (*transactionModel, error) {
	query := `SELECT * FROM ` + tableNameTx + ` WHERE signature = $1;`
	res := &transactionModel{}
	err := db.GetContext(ctx, res, query, sig)

	return res, pg.CheckNoRows(err, transaction.ErrNotFound)
}

func dbGetFirstPending(ctx context.Context, db *sqlx.DB, address string) (*transaction.Record, error) {
	// todo: maybe merge this with dbGetAllByAddress

	query := `
		SELECT 
			tx.id, signature, block_id, block_time, raw_data, fee, has_errors,
			confirmation_state, confirmations, created_at, account, pre_balance, post_balance
		FROM ` + tableNameTx + ` as tx INNER JOIN ` + tableNameTxBalance + ` as txb
		ON (account = $1 AND tx.signature = txb.transaction_id)
	`
	query = query + "WHERE tx.confirmation_state < 3 ORDER BY block_id ASC, tx.id ASC LIMIT 1"

	// Run the query
	rows, err := db.QueryxContext(ctx, query, address)
	if err != nil {
		// TODO: this is technically swallowing the error
		return nil, pg.CheckNoRows(err, transaction.ErrNotFound)
	}

	var tx *transaction.Record
	for rows.Next() {
		var (
			txId              sql.NullInt64 //`db:"id"`
			signature         string        //`db:"signature"`
			slot              uint64        //`db:"block_id"`
			blockTime         time.Time     //`db:"block_time"`
			data              []byte        //`db:"raw_data"`
			fee               sql.NullInt64 //`db:"fee"`
			hasErrors         bool          //`db:"has_errors"`
			confirmationState uint          //`db:"confirmation_state"`
			confirmations     uint          //`db:"confirmations"`
			createdAt         time.Time     //`db:"created_at"`
			account           string        //`db:"account"`
			preBalance        uint64        //`db:"pre_balance"`
			postBalance       uint64        //`db:"post_balance"`
		)

		err = rows.Scan(
			&txId,
			&signature,
			&slot,
			&blockTime,
			&data, &fee,
			&hasErrors,
			&confirmationState,
			&confirmations,
			&createdAt,
			&account,
			&preBalance,
			&postBalance,
		)
		if err != nil {
			return nil, err
		}

		tb := &transaction.TokenBalance{
			Account:     account,
			PreBalance:  preBalance,
			PostBalance: postBalance,
		}

		// Create the nested transaction.Result
		if tx != nil {
			tx.TokenBalances = append(tx.TokenBalances, tb)
		} else {
			txModel := &transactionModel{
				Id:                txId,
				Signature:         signature,
				Slot:              slot,
				BlockTime:         blockTime,
				Data:              data,
				Fee:               fee,
				HasErrors:         hasErrors,
				ConfirmationState: confirmationState,
				Confirmations:     confirmations,
				CreatedAt:         createdAt,
			}
			tx = fromTxModel(txModel)
			tx.TokenBalances = []*transaction.TokenBalance{tb}
		}
	}

	if tx == nil {
		return nil, transaction.ErrNotFound
	}

	return tx, nil
}

func dbGetLatestByState(ctx context.Context, db *sqlx.DB, address string, state transaction.Confirmation) (*transaction.Record, error) {
	// todo: maybe merge this with dbGetAllByAddress

	query := `
		SELECT 
			tx.id, signature, block_id, block_time, raw_data, fee, has_errors,
			confirmation_state, confirmations, created_at, account, pre_balance, post_balance
		FROM ` + tableNameTx + ` as tx INNER JOIN ` + tableNameTxBalance + ` as txb
		ON (account = $1 AND tx.signature = txb.transaction_id)
	`
	query = query + "WHERE tx.confirmation_state = $2 ORDER BY block_id DESC, tx.id DESC LIMIT 1"

	// Run the query
	rows, err := db.QueryxContext(ctx, query, address, state)
	if err != nil {
		// TODO: this is technically swallowing the error
		return nil, pg.CheckNoRows(err, transaction.ErrNotFound)
	}

	var tx *transaction.Record
	for rows.Next() {
		var (
			txId              sql.NullInt64 //`db:"id"`
			signature         string        //`db:"signature"`
			slot              uint64        //`db:"block_id"`
			blockTime         time.Time     //`db:"block_time"`
			data              []byte        //`db:"raw_data"`
			fee               sql.NullInt64 //`db:"fee"`
			hasErrors         bool          //`db:"has_errors"`
			confirmationState uint          //`db:"confirmation_state"`
			confirmations     uint          //`db:"confirmations"`
			createdAt         time.Time     //`db:"created_at"`
			account           string        //`db:"account"`
			preBalance        uint64        //`db:"pre_balance"`
			postBalance       uint64        //`db:"post_balance"`
		)

		err = rows.Scan(
			&txId,
			&signature,
			&slot,
			&blockTime,
			&data,
			&fee,
			&hasErrors,
			&confirmationState,
			&confirmations,
			&createdAt,
			&account,
			&preBalance,
			&postBalance,
		)
		if err != nil {
			return nil, err
		}

		tb := &transaction.TokenBalance{
			Account:     account,
			PreBalance:  preBalance,
			PostBalance: postBalance,
		}

		// Create the nested transaction.Result
		if tx != nil {
			tx.TokenBalances = append(tx.TokenBalances, tb)
		} else {
			txModel := &transactionModel{
				Id:                txId,
				Signature:         signature,
				Slot:              slot,
				BlockTime:         blockTime,
				Data:              data,
				Fee:               fee,
				HasErrors:         hasErrors,
				ConfirmationState: confirmationState,
				Confirmations:     confirmations,
				CreatedAt:         createdAt,
			}
			tx = fromTxModel(txModel)
			tx.TokenBalances = []*transaction.TokenBalance{tb}
		}
	}

	if tx == nil {
		return nil, transaction.ErrNotFound
	}

	return tx, nil
}

func dbGetAllByAddress(ctx context.Context, db *sqlx.DB, address string, cursor uint64, limit uint, ordering q.Ordering) ([]*transaction.Record, error) {
	query := `
		SELECT 
			tx.id, signature, block_id, block_time, raw_data, fee, has_errors,
			confirmation_state, confirmations, created_at, account, pre_balance, post_balance
		FROM ` + tableNameTx + ` as tx INNER JOIN ` + tableNameTxBalance + ` as txb
		ON (account = $1 AND tx.signature = txb.transaction_id)
	`

	// Check if we need to add the cursor condition
	withCursor := cursor > 0
	if withCursor {
		if ordering == q.Ascending {
			query = query + "WHERE tx.id > $3"
		} else {
			query = query + "WHERE tx.id < $3"
		}
	}

	// Add the ordering and limit
	order := q.FromOrderingWithFallback(ordering, "ASC")

	// Note, we can't use the block_id for order as it might be absent on recent transactions
	query = query + " ORDER BY id " + order
	query = query + " LIMIT $2"

	args := []interface{}{address, limit}
	if withCursor {
		args = append(args, cursor)
	}

	// Run the query
	rows, err := db.QueryxContext(ctx, query, args...)
	if err != nil {
		// TODO: this is technically swallowing the error
		return nil, pg.CheckNoRows(err, transaction.ErrNotFound)
	}

	var txs = make(map[string]*transaction.Record)
	res := []*transaction.Record{}

	for rows.Next() {
		var (
			txId              sql.NullInt64 //`db:"id"`
			signature         string        //`db:"signature"`
			slot              uint64        //`db:"block_id"`
			blockTime         time.Time     //`db:"block_time"`
			data              []byte        //`db:"raw_data"`
			fee               sql.NullInt64 //`db:"fee"`
			hasErrors         bool          //`db:"has_errors"`
			confirmationState uint          //`db:"confirmation_state"`
			confirmations     uint          //`db:"confirmations"`
			createdAt         time.Time     //`db:"created_at"`
			account           string        //`db:"account"`
			preBalance        uint64        //`db:"pre_balance"`
			postBalance       uint64        //`db:"post_balance"`
		)

		err = rows.Scan(
			&txId,
			&signature,
			&slot,
			&blockTime,
			&data,
			&fee,
			&hasErrors,
			&confirmationState,
			&confirmations,
			&createdAt,
			&account,
			&preBalance,
			&postBalance,
		)
		if err != nil {
			return nil, err
		}

		tb := &transaction.TokenBalance{
			Account:     account,
			PreBalance:  preBalance,
			PostBalance: postBalance,
		}

		// Create the nested transaction.Result
		if tx, ok := txs[signature]; ok {
			tx.TokenBalances = append(tx.TokenBalances, tb)
		} else {
			txModel := &transactionModel{
				Id:                txId,
				Signature:         signature,
				Slot:              slot,
				BlockTime:         blockTime,
				Data:              data,
				Fee:               fee,
				HasErrors:         hasErrors,
				ConfirmationState: confirmationState,
				Confirmations:     confirmations,
				CreatedAt:         createdAt,
			}
			tx = fromTxModel(txModel)
			tx.TokenBalances = []*transaction.TokenBalance{tb}
			txs[signature] = tx

			res = append(res, tx)
		}
	}

	if len(res) == 0 {
		return nil, transaction.ErrNotFound
	}

	return res, nil
}

func dbGetSignaturesByState(ctx context.Context, db *sqlx.DB, state transaction.Confirmation, limit uint, ordering q.Ordering) ([]string, error) {
	res := []string{}
	err := db.SelectContext(ctx, &res,
		makeAllQuery(tableNameTx, "signature", "confirmation_state = $2", ordering),
		limit, state,
	)

	if err != nil {
		return nil, pg.CheckNoRows(err, transaction.ErrNotFound)
	}
	if res == nil {
		return nil, transaction.ErrNotFound
	}

	return res, nil
}

type tokenBalanceModel struct {
	Id            sql.NullInt64 `db:"id"`
	TransactionId string        `db:"transaction_id"`
	Account       string        `db:"account"`
	PreBalance    uint64        `db:"pre_balance"`
	PostBalance   uint64        `db:"post_balance"`
}

func toTxBalanceModel(obj *transaction.TokenBalance) *tokenBalanceModel {
	return &tokenBalanceModel{
		Account:     obj.Account,
		PreBalance:  obj.PreBalance,
		PostBalance: obj.PostBalance,
	}
}

func fromTxBalanceModel(obj *tokenBalanceModel) *transaction.TokenBalance {
	return &transaction.TokenBalance{
		Account:     obj.Account,
		PreBalance:  obj.PreBalance,
		PostBalance: obj.PostBalance,
	}
}

func (self *tokenBalanceModel) txSaveTxBalance(ctx context.Context, tx *sqlx.Tx) error {
	query := `INSERT INTO ` + tableNameTxBalance + `
			(transaction_id, account, pre_balance, post_balance)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (transaction_id, account)
		DO UPDATE
			SET (pre_balance, post_balance) = ($3,$4)
			WHERE ` + tableNameTxBalance + `.transaction_id = $1 AND ` + tableNameTxBalance + `.account = $2
		RETURNING *;`

	err := tx.QueryRowxContext(ctx, query,
		self.TransactionId,
		self.Account,
		self.PreBalance,
		self.PostBalance,
	).StructScan(self)

	return err
}

func dbGetAllTxBalance(ctx context.Context, db *sqlx.DB, sig string) ([]*tokenBalanceModel, error) {
	query := `SELECT * FROM ` + tableNameTxBalance + ` WHERE transaction_id = $1 ORDER BY id;`

	var res []*tokenBalanceModel
	err := db.SelectContext(ctx, &res, query, sig)

	return res, pg.CheckNoRows(err, transaction.ErrNotFound)
}
