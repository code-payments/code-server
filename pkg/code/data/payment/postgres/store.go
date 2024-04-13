package postgres

import (
	"context"
	"database/sql"

	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/jmoiron/sqlx"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) payment.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

func (s *store) Get(ctx context.Context, txId string, index uint32) (*payment.Record, error) {
	obj, err := dbGet(ctx, s.db, txId, index)
	if err != nil {
		return nil, err
	}

	return fromModel(obj), nil
}

func (s *store) Put(ctx context.Context, data *payment.Record) error {
	return toModel(data).dbSave(ctx, s.db)
}

func (s *store) Update(ctx context.Context, data *payment.Record) error {
	return toModel(data).dbUpdate(ctx, s.db)
}

func (s *store) GetAllForTransaction(ctx context.Context, txId string) ([]*payment.Record, error) {
	list, err := dbGetAllForTransaction(ctx, s.db, txId)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetAllForAccount(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering) ([]*payment.Record, error) {
	list, err := dbGetAllForAccount(ctx, s.db, account, cursor, limit, ordering)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetAllForAccountByType(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) ([]*payment.Record, error) {
	list, err := dbGetAllForAccountByType(ctx, s.db, account, cursor, limit, ordering, paymentType)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetAllForAccountByTypeAfterBlock(ctx context.Context, account string, block uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) ([]*payment.Record, error) {
	list, err := dbGetAllForAccountByTypeAfterBlock(ctx, s.db, account, block, cursor, limit, ordering, paymentType)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetAllForAccountByTypeWithinBlockRange(ctx context.Context, account string, lowerBound, upperBound uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) ([]*payment.Record, error) {
	list, err := dbGetAllForAccountByTypeWithinBlockRange(ctx, s.db, account, lowerBound, upperBound, cursor, limit, ordering, paymentType)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetAllExternalDepositsAfterBlock(ctx context.Context, account string, block uint64, cursor uint64, limit uint, ordering query.Ordering) ([]*payment.Record, error) {
	list, err := dbGetAllExternalDepositsAfterBlock(ctx, s.db, account, block, cursor, limit, ordering)
	if err != nil {
		return nil, err
	}

	res := []*payment.Record{}
	for _, item := range list {
		res = append(res, fromModel(item))
	}

	return res, nil
}

func (s *store) GetExternalDepositAmount(ctx context.Context, account string) (uint64, error) {
	return dbGetExternalDepositAmount(ctx, s.db, account)
}
