package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/data/account"
	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/pointer"
)

const (
	tableName = "codewallet__core_accountinfov2"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	OwnerAccount     string `db:"owner_account"`
	AuthorityAccount string `db:"authority_account"`
	TokenAccount     string `db:"token_account"`
	MintAccount      string `db:"mint_account"`

	AccountType    int    `db:"account_type"`
	Index          uint64 `db:"index"`
	RelationshipTo string `db:"relationship_to"`

	RequiresDepositSync  bool      `db:"requires_deposit_sync"`
	DepositsLastSyncedAt time.Time `db:"deposits_last_synced_at"`

	RequiresAutoReturnCheck bool `db:"requires_auto_return_check"`

	RequiresSwapRetry bool      `db:"requires_swap_retry"`
	LastSwapRetryAt   time.Time `db:"last_swap_retry_at"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *account.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		OwnerAccount:     obj.OwnerAccount,
		AuthorityAccount: obj.AuthorityAccount,
		TokenAccount:     obj.TokenAccount,
		MintAccount:      obj.MintAccount,

		AccountType:    int(obj.AccountType),
		Index:          obj.Index,
		RelationshipTo: *pointer.StringOrDefault(obj.RelationshipTo, ""),

		RequiresDepositSync:  obj.RequiresDepositSync,
		DepositsLastSyncedAt: obj.DepositsLastSyncedAt.UTC(),

		RequiresAutoReturnCheck: obj.RequiresAutoReturnCheck,

		RequiresSwapRetry: obj.RequiresSwapRetry,
		LastSwapRetryAt:   obj.LastSwapRetryAt,

		CreatedAt: obj.CreatedAt.UTC(),
	}, nil
}

func fromModel(obj *model) *account.Record {
	return &account.Record{
		Id: uint64(obj.Id.Int64),

		OwnerAccount:     obj.OwnerAccount,
		AuthorityAccount: obj.AuthorityAccount,
		TokenAccount:     obj.TokenAccount,
		MintAccount:      obj.MintAccount,

		AccountType:    commonpb.AccountType(obj.AccountType),
		Index:          obj.Index,
		RelationshipTo: pointer.StringIfValid(len(obj.RelationshipTo) > 0, obj.RelationshipTo),

		RequiresDepositSync:  obj.RequiresDepositSync,
		DepositsLastSyncedAt: obj.DepositsLastSyncedAt,

		RequiresAutoReturnCheck: obj.RequiresAutoReturnCheck,

		RequiresSwapRetry: obj.RequiresSwapRetry,
		LastSwapRetryAt:   obj.LastSwapRetryAt,

		CreatedAt: obj.CreatedAt,
	}
}

func (m *model) dbInsert(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		if m.CreatedAt.IsZero() {
			m.CreatedAt = time.Now()
		}

		query := `INSERT INTO ` + tableName + `
			(owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at
		`
		err := tx.QueryRowxContext(
			ctx,
			query,
			m.OwnerAccount,
			m.AuthorityAccount,
			m.TokenAccount,
			m.MintAccount,
			m.AccountType,
			m.Index,
			m.RelationshipTo,
			m.RequiresDepositSync,
			m.DepositsLastSyncedAt,
			m.RequiresAutoReturnCheck,
			m.RequiresSwapRetry,
			m.LastSwapRetryAt,
			m.CreatedAt,
		).StructScan(m)
		if err == nil {
			return nil
		}

		// There are multiple unique violations, which may indicate we have something
		// invalid or the record exists as is. We need to query it to see what's
		// up and return the right error code.
		if pgutil.IsUniqueViolation(err) {
			existingModel, err := dbGetByTokenAddress(ctx, db, m.TokenAccount)
			if err == account.ErrAccountInfoNotFound {
				return account.ErrInvalidAccountInfo
			} else if err != nil {
				return err
			}

			if equivalentModels(existingModel, m) {
				return account.ErrAccountInfoExists
			}
			return account.ErrInvalidAccountInfo
		}

		return err
	})
}

func (m *model) dbUpdate(ctx context.Context, db *sqlx.DB) error {
	query := `UPDATE ` + tableName + `
		SET requires_deposit_sync = $2, deposits_last_synced_at = $3, requires_auto_return_check = $4, requires_swap_retry = $5, last_swap_retry_at = $6
		WHERE token_account = $1
		RETURNING id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.TokenAccount,
		m.RequiresDepositSync,
		m.DepositsLastSyncedAt,
		m.RequiresAutoReturnCheck,
		m.RequiresSwapRetry,
		m.LastSwapRetryAt,
	).StructScan(m)

	if err != nil {
		return pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	return nil
}

func dbGetByTokenAddress(ctx context.Context, db *sqlx.DB, address string) (*model, error) {
	res := &model{}

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE token_account = $1
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		address,
	).StructScan(res)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	return res, nil
}

func dbGetByTokenAddressBatch(ctx context.Context, db *sqlx.DB, addresses ...string) ([]*model, error) {
	res := []*model{}

	individualFilters := make([]string, len(addresses))
	for i, address := range addresses {
		individualFilters[i] = fmt.Sprintf("'%s'", address)
	}

	query := fmt.Sprintf(
		`SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM `+tableName+`
		WHERE token_account IN (%s)`,
		strings.Join(individualFilters, ", "),
	)

	err := db.SelectContext(ctx, &res, query)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	if len(res) != len(addresses) {
		return nil, account.ErrAccountInfoNotFound
	}
	return res, nil
}

func dbGetByAuthorityAddress(ctx context.Context, db *sqlx.DB, address string) (*model, error) {
	res := &model{}

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE authority_account = $1
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		address,
	).StructScan(res)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	return res, nil
}

func dbGetLatestByOwnerAddress(ctx context.Context, db *sqlx.DB, address string) ([]*model, error) {
	var res1 []*model

	query1 := `SELECT DISTINCT ON (account_type, relationship_to) id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE owner_account = $1 AND account_type NOT IN ($2)
		ORDER BY account_type, relationship_to, index DESC
	`

	err := db.SelectContext(
		ctx,
		&res1,
		query1,
		address,
		commonpb.AccountType_POOL,
	)
	if err != nil && !pgutil.IsNoRows(err) {
		return nil, err
	}

	var res2 []*model

	query2 := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE owner_account = $1 AND account_type IN ($2)
		ORDER BY index ASC
	`
	err = db.SelectContext(
		ctx,
		&res2,
		query2,
		address,
		commonpb.AccountType_POOL,
	)
	if err != nil && !pgutil.IsNoRows(err) {
		return nil, err
	}

	var res []*model
	res = append(res, res1...)
	res = append(res, res2...)
	if len(res) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}
	return res, nil
}

func dbGetLatestByOwnerAddressAndType(ctx context.Context, db *sqlx.DB, address string, accountType commonpb.AccountType) (*model, error) {
	if accountType == commonpb.AccountType_RELATIONSHIP {
		return nil, errors.New("relationship account not supported")
	}

	res := &model{}

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE owner_account = $1 AND account_type = $2
		ORDER BY index DESC
		LIMIT 1
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		address,
		accountType,
	).StructScan(res)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	return res, nil
}

func dbGetRelationshipByOwnerAddress(ctx context.Context, db *sqlx.DB, address, relationshipTo string) (*model, error) {
	res := &model{}

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE owner_account = $1 AND account_type = $2 AND index = 0 AND relationship_to = $3
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		address,
		commonpb.AccountType_RELATIONSHIP,
		relationshipTo,
	).StructScan(res)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}
	return res, nil
}

func dbGetPrioritizedRequiringDepositSync(ctx context.Context, db *sqlx.DB, limit uint64) ([]*model, error) {
	var res []*model

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE requires_deposit_sync = TRUE
		ORDER BY deposits_last_synced_at ASC
		LIMIT $1
	`
	err := db.SelectContext(
		ctx,
		&res,
		query,
		limit,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}

	if len(res) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}
	return res, nil
}

func dbCountRequiringDepositSync(ctx context.Context, db *sqlx.DB) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE requires_deposit_sync = TRUE
	`

	err := db.GetContext(ctx, &res, query)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbGetPrioritizedRequiringAutoReturnChecks(ctx context.Context, db *sqlx.DB, minAge time.Duration, limit uint64) ([]*model, error) {
	var res []*model

	query := `SELECT id, owner_account, authority_account, token_account, mint_account, account_type, index, relationship_to, requires_deposit_sync, deposits_last_synced_at, requires_auto_return_check, requires_swap_retry, last_swap_retry_at, created_at FROM ` + tableName + `
		WHERE requires_auto_return_check = TRUE AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2
	`
	err := db.SelectContext(
		ctx,
		&res,
		query,
		time.Now().Add(-minAge),
		limit,
	)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, account.ErrAccountInfoNotFound)
	}

	if len(res) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}
	return res, nil
}

func dbCountRequiringAutoReturnCheck(ctx context.Context, db *sqlx.DB) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + tableName + `
		WHERE requires_auto_return_check = TRUE
	`

	err := db.GetContext(ctx, &res, query)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func equivalentModels(obj1, obj2 *model) bool {
	if obj1.OwnerAccount != obj2.OwnerAccount {
		return false
	}

	if obj1.AuthorityAccount != obj2.AuthorityAccount {
		return false
	}

	if obj1.TokenAccount != obj2.TokenAccount {
		return false
	}

	if obj1.MintAccount != obj2.MintAccount {
		return false
	}

	if obj1.Index != obj2.Index {
		return false
	}

	if obj1.AccountType != obj2.AccountType {
		return false
	}

	if obj1.RelationshipTo != obj2.RelationshipTo {
		return false
	}

	return true
}
