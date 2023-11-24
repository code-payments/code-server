package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	pg "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) transaction.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s *store) Put(ctx context.Context, record *transaction.Record) error {
	// Be careful in this func, the "tx" variable refers to a database
	// transaction (bulk insert), not the incoming solana transaction record

	return pg.ExecuteInTx(ctx, s.db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		err := toTxModel(record).txSaveTx(ctx, tx)
		if err != nil {
			return err
		}

		for _, item := range record.TokenBalances {
			txBalanceModel := toTxBalanceModel(item)
			txBalanceModel.TransactionId = record.Signature

			err := txBalanceModel.txSaveTxBalance(ctx, tx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *store) Get(ctx context.Context, sig string) (*transaction.Record, error) {
	txModel, err := dbGetTx(ctx, s.db, sig)
	if err != nil {
		return nil, err
	}

	txBalances, err := dbGetAllTxBalance(ctx, s.db, sig)
	if err != nil {
		return nil, err
	}

	tx := fromTxModel(txModel)
	for _, item := range txBalances {
		tx.TokenBalances = append(tx.TokenBalances, fromTxBalanceModel(item))
	}

	return tx, nil
}

func (s *store) GetAllByAddress(ctx context.Context, address string, cursor uint64, limit uint, ordering query.Ordering) ([]*transaction.Record, error) {
	res, err := dbGetAllByAddress(ctx, s.db, address, cursor, limit, ordering)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (s *store) GetLatestByState(ctx context.Context, address string, state transaction.Confirmation) (*transaction.Record, error) {
	return dbGetLatestByState(ctx, s.db, address, state)
}

func (s *store) GetFirstPending(ctx context.Context, address string) (*transaction.Record, error) {
	return dbGetFirstPending(ctx, s.db, address)
}

func (s *store) GetSignaturesByState(ctx context.Context, filter transaction.Confirmation, limit uint, ordering query.Ordering) ([]string, error) {
	return dbGetSignaturesByState(ctx, s.db, filter, limit, ordering)
}
