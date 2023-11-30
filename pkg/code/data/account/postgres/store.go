package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/data/account"
)

type store struct {
	db *sqlx.DB
}

func New(db *sql.DB) account.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Put implements account.Store.Put
func (s *store) Put(ctx context.Context, record *account.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbInsert(ctx, s.db)
	if err != nil {
		return err
	}

	updated := fromModel(model)
	updated.CopyTo(record)

	return nil
}

// Update implements account.Store.Update
func (s *store) Update(ctx context.Context, record *account.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	err = model.dbUpdate(ctx, s.db)
	if err != nil {
		return err
	}

	updated := fromModel(model)
	updated.CopyTo(record)

	return nil
}

// GetByTokenAddress implements account.Store.GetByTokenAddress
func (s *store) GetByTokenAddress(ctx context.Context, address string) (*account.Record, error) {
	model, err := dbGetByTokenAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetByAuthorityAddress implements account.Store.GetByAuthorityAddress
func (s *store) GetByAuthorityAddress(ctx context.Context, address string) (*account.Record, error) {
	model, err := dbGetByAuthorityAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetLatestByOwnerAddress implements account.Store.GetLatestByOwnerAddress
func (s *store) GetLatestByOwnerAddress(ctx context.Context, address string) (map[commonpb.AccountType][]*account.Record, error) {
	modelsByType, err := dbGetLatestByOwnerAddress(ctx, s.db, address)
	if err != nil {
		return nil, err
	}

	res := make(map[commonpb.AccountType][]*account.Record)
	for _, model := range modelsByType {
		record := fromModel(model)
		res[record.AccountType] = append(res[record.AccountType], record)
	}
	return res, nil
}

// GetLatestByOwnerAddressAndType implements account.Store.GetLatestByOwnerAddressAndType
func (s *store) GetLatestByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (*account.Record, error) {
	model, err := dbGetLatestByOwnerAddressAndType(ctx, s.db, address, accountType)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetRelationshipByOwnerAddress implements account.Store.GetRelationshipByOwnerAddress
func (s *store) GetRelationshipByOwnerAddress(ctx context.Context, address, relationshipTo string) (*account.Record, error) {
	model, err := dbGetRelationshipByOwnerAddress(ctx, s.db, address, relationshipTo)
	if err != nil {
		return nil, err
	}
	return fromModel(model), nil
}

// GetPrioritizedRequiringDepositSync implements account.Store.GetPrioritizedRequiringDepositSync
func (s *store) GetPrioritizedRequiringDepositSync(ctx context.Context, limit uint64) ([]*account.Record, error) {
	models, err := dbGetPrioritizedRequiringDepositSync(ctx, s.db, limit)
	if err != nil {
		return nil, err
	}

	res := make([]*account.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}

// CountRequiringDepositSync implements account.Store.CountRequiringDepositSync
func (s *store) CountRequiringDepositSync(ctx context.Context) (uint64, error) {
	return dbCountRequiringDepositSync(ctx, s.db)
}

// GetPrioritizedRequiringAutoReturnCheck implements account.Store.GetPrioritizedRequiringAutoReturnCheck
func (s *store) GetPrioritizedRequiringAutoReturnCheck(ctx context.Context, minAge time.Duration, limit uint64) ([]*account.Record, error) {
	models, err := dbGetPrioritizedRequiringAutoReturnChecks(ctx, s.db, minAge, limit)
	if err != nil {
		return nil, err
	}

	res := make([]*account.Record, len(models))
	for i, model := range models {
		res[i] = fromModel(model)
	}
	return res, nil
}

// CountRequiringAutoReturnCheck implements account.Store.CountRequiringAutoReturnCheck
func (s *store) CountRequiringAutoReturnCheck(ctx context.Context) (uint64, error) {
	return dbCountRequiringAutoReturnCheck(ctx, s.db)
}
