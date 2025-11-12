package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/currency"
	q "github.com/code-payments/code-server/pkg/database/query"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	exchangeRateTableName = "codewallet__core_exchangerate"
	metadataTableName     = "codewallet__core_currencymetadata"
	reserveTableName      = "codewallet__core_currencyreserve"

	dateFormat = "2006-01-02"
)

type exchangeRateModel struct {
	Id           sql.NullInt64 `db:"id"`
	ForDate      string        `db:"for_date"`
	ForTimestamp time.Time     `db:"for_timestamp"`
	CurrencyCode string        `db:"currency_code"`
	CurrencyRate float64       `db:"currency_rate"`
}

func toExchangeRateModel(obj *currency.ExchangeRateRecord) *exchangeRateModel {
	return &exchangeRateModel{
		Id:           sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		ForDate:      obj.Time.UTC().Format(dateFormat),
		ForTimestamp: obj.Time.UTC(),
		CurrencyCode: obj.Symbol,
		CurrencyRate: obj.Rate,
	}
}

func fromExchangeRateModel(obj *exchangeRateModel) *currency.ExchangeRateRecord {
	return &currency.ExchangeRateRecord{
		Id:     uint64(obj.Id.Int64),
		Time:   obj.ForTimestamp.UTC(),
		Symbol: obj.CurrencyCode,
		Rate:   obj.CurrencyRate,
	}
}

type metadataModel struct {
	Id sql.NullInt64 `db:"id"`

	Name        string `db:"name"`
	Symbol      string `db:"symbol"`
	Description string `db:"description"`
	ImageUrl    string `db:"image_url"`

	Seed string `db:"seed"`

	Authority string `db:"authority"`

	Mint     string `db:"mint"`
	MintBump uint8  `db:"mint_bump"`
	Decimals uint8  `db:"decimals"`

	CurrencyConfig     string `db:"currency_config"`
	CurrencyConfigBump uint8  `db:"currency_config_bump"`

	LiquidityPool     string `db:"liquidity_pool"`
	LiquidityPoolBump uint8  `db:"liquidity_pool_bump"`

	VaultMint     string `db:"vault_mint"`
	VaultMintBump uint8  `db:"vault_mint_bump"`

	VaultCore     string `db:"vault_core"`
	VaultCoreBump uint8  `db:"vault_core_bump"`

	FeesMint  string `db:"fees_mint"`
	BuyFeeBps uint16 `db:"buy_fee_bps"`

	FeesCore   string `db:"fees_core"`
	SellFeeBps uint16 `db:"sell_fee_bps"`

	Alt string `db:"alt"`

	CreatedBy string    `db:"created_by"`
	CreatedAt time.Time `db:"created_at"`
}

func toMetadataModel(obj *currency.MetadataRecord) (*metadataModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &metadataModel{
		Id: sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},

		Name:        obj.Name,
		Symbol:      obj.Symbol,
		Description: obj.Description,
		ImageUrl:    obj.ImageUrl,

		Seed: obj.Seed,

		Authority: obj.Authority,

		Mint:     obj.Mint,
		MintBump: obj.MintBump,
		Decimals: obj.Decimals,

		CurrencyConfig:     obj.CurrencyConfig,
		CurrencyConfigBump: obj.CurrencyConfigBump,

		LiquidityPool:     obj.LiquidityPool,
		LiquidityPoolBump: obj.LiquidityPoolBump,

		VaultMint:     obj.VaultMint,
		VaultMintBump: obj.VaultMintBump,

		VaultCore:     obj.VaultCore,
		VaultCoreBump: obj.VaultCoreBump,

		FeesMint:  obj.FeesMint,
		BuyFeeBps: obj.BuyFeeBps,

		FeesCore:   obj.FeesCore,
		SellFeeBps: obj.SellFeeBps,

		Alt: obj.Alt,

		CreatedBy: obj.CreatedBy,
		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromMetadataModel(obj *metadataModel) *currency.MetadataRecord {
	return &currency.MetadataRecord{
		Id: uint64(obj.Id.Int64),

		Name:        obj.Name,
		Symbol:      obj.Symbol,
		Description: obj.Description,
		ImageUrl:    obj.ImageUrl,

		Seed: obj.Seed,

		Authority: obj.Authority,

		Mint:     obj.Mint,
		MintBump: obj.MintBump,
		Decimals: obj.Decimals,

		CurrencyConfig:     obj.CurrencyConfig,
		CurrencyConfigBump: obj.CurrencyConfigBump,

		LiquidityPool:     obj.LiquidityPool,
		LiquidityPoolBump: obj.LiquidityPoolBump,

		VaultMint:     obj.VaultMint,
		VaultMintBump: obj.VaultMintBump,

		VaultCore:     obj.VaultCore,
		VaultCoreBump: obj.VaultCoreBump,

		FeesMint:  obj.FeesMint,
		BuyFeeBps: obj.BuyFeeBps,

		FeesCore:   obj.FeesCore,
		SellFeeBps: obj.SellFeeBps,

		Alt: obj.Alt,

		CreatedBy: obj.CreatedBy,
		CreatedAt: obj.CreatedAt,
	}
}

type reserveModel struct {
	Id                sql.NullInt64 `db:"id"`
	ForDate           string        `db:"for_date"`
	ForTimestamp      time.Time     `db:"for_timestamp"`
	Mint              string        `db:"mint"`
	SupplyFromBonding uint64        `db:"supply_from_bonding"`
	CoreMintLocked    uint64        `db:"core_mint_locked"`
}

func toReserveModel(obj *currency.ReserveRecord) (*reserveModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &reserveModel{
		Id:                sql.NullInt64{Int64: int64(obj.Id), Valid: obj.Id > 0},
		ForDate:           obj.Time.UTC().Format(dateFormat),
		ForTimestamp:      obj.Time.UTC(),
		Mint:              obj.Mint,
		SupplyFromBonding: obj.SupplyFromBonding,
		CoreMintLocked:    obj.CoreMintLocked,
	}, nil
}

func fromReserveModel(obj *reserveModel) *currency.ReserveRecord {
	return &currency.ReserveRecord{
		Id:                uint64(obj.Id.Int64),
		Time:              obj.ForTimestamp.UTC(),
		Mint:              obj.Mint,
		SupplyFromBonding: obj.SupplyFromBonding,
		CoreMintLocked:    obj.CoreMintLocked,
	}
}

func makeTimeBasedSelectQuery(table, condition string, ordering q.Ordering) string {
	return `SELECT * FROM ` + table + ` WHERE ` + condition + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
}

func makeTimeBasedGetQuery(table, condition string, ordering q.Ordering) string {
	return makeTimeBasedSelectQuery(table, condition, ordering) + ` LIMIT 1`
}

func makeTimeBasedRangeQuery(table, condition string, ordering q.Ordering, interval q.Interval) string {
	var query, bucket string

	if interval == q.IntervalRaw {
		query = `SELECT *`
	} else {
		bucket = `date_trunc('` + q.FromIntervalWithFallback(interval, "hour") + `', for_timestamp)`
		query = `SELECT DISTINCT ON (` + bucket + `) *`
	}

	query = query + ` FROM ` + table + ` WHERE ` + condition

	if interval == q.IntervalRaw {
		query = query + ` ORDER BY for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")
	} else {
		query = query + ` ORDER BY ` + bucket + `, for_timestamp DESC` // keep only the latest record for each bucket
	}

	return query
}

func (m *exchangeRateModel) txSave(ctx context.Context, tx *sqlx.Tx) error {
	err := tx.QueryRowxContext(ctx,
		`INSERT INTO `+exchangeRateTableName+`
		(for_date, for_timestamp, currency_code, currency_rate)
		VALUES ($1, $2, $3, $4)
		RETURNING id, for_date, for_timestamp, currency_code, currency_rate`,
		m.ForDate,
		m.ForTimestamp,
		m.CurrencyCode,
		m.CurrencyRate,
	).StructScan(m)

	return pgutil.CheckUniqueViolation(err, currency.ErrExists)
}

func (m *metadataModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		err := tx.QueryRowxContext(ctx,
			`INSERT INTO `+metadataTableName+`
			(name, symbol, description, image_url, seed, authority, mint, mint_bump, decimals, currency_config, currency_config_bump, liquidity_pool, liquidity_pool_bump, vault_mint, vault_mint_bump, vault_core, vault_core_bump, fees_mint, buy_fee_bps, fees_core, sell_fee_bps, alt, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
			RETURNING id, name, symbol, description, image_url, seed, authority, mint, mint_bump, decimals, currency_config, currency_config_bump, liquidity_pool, liquidity_pool_bump, vault_mint, vault_mint_bump, vault_core, vault_core_bump, fees_mint, buy_fee_bps, fees_core, sell_fee_bps, alt, created_by, created_at`,
			m.Name,
			m.Symbol,
			m.Description,
			m.ImageUrl,
			m.Seed,
			m.Authority,
			m.Mint,
			m.MintBump,
			m.Decimals,
			m.CurrencyConfig,
			m.CurrencyConfigBump,
			m.LiquidityPool,
			m.LiquidityPoolBump,
			m.VaultMint,
			m.VaultMintBump,
			m.VaultCore,
			m.VaultCoreBump,
			m.FeesMint,
			m.BuyFeeBps,
			m.FeesCore,
			m.SellFeeBps,
			m.Alt,
			m.CreatedBy,
			m.CreatedAt,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, currency.ErrExists)
	})
}

func (m *reserveModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	return pgutil.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		err := tx.QueryRowxContext(ctx,
			`INSERT INTO `+reserveTableName+`
			(for_date, for_timestamp, mint, supply_from_bonding, core_mint_locked)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, for_date, for_timestamp, mint, supply_from_bonding, core_mint_locked`,
			m.ForDate,
			m.ForTimestamp,
			m.Mint,
			m.SupplyFromBonding,
			m.CoreMintLocked,
		).StructScan(m)

		return pgutil.CheckUniqueViolation(err, currency.ErrExists)
	})
}

func dbGetExchangeRateBySymbolAndTime(ctx context.Context, db *sqlx.DB, symbol string, t time.Time, ordering q.Ordering) (*exchangeRateModel, error) {
	res := &exchangeRateModel{}
	err := db.GetContext(ctx, res,
		makeTimeBasedGetQuery(exchangeRateTableName, "currency_code = $1 AND for_date = $2 AND for_timestamp <= $3", ordering),
		symbol,
		t.UTC().Format(dateFormat),
		t.UTC(),
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}

func dbGetAllExchangeRatesByTime(ctx context.Context, db *sqlx.DB, t time.Time, ordering q.Ordering) ([]*exchangeRateModel, error) {
	query := `SELECT DISTINCT ON (currency_code) *
		FROM ` + exchangeRateTableName + `
		WHERE for_date = $1 AND for_timestamp <= $2
		ORDER BY currency_code, for_timestamp ` + q.FromOrderingWithFallback(ordering, "asc")

	res := []*exchangeRateModel{}
	err := db.SelectContext(ctx, &res, query, t.UTC().Format(dateFormat), t.UTC())

	if err != nil {
		return nil, pgutil.CheckNoRows(err, currency.ErrNotFound)
	}
	if res == nil {
		return nil, currency.ErrNotFound
	}
	if len(res) == 0 {
		return nil, currency.ErrNotFound
	}

	return res, nil
}

func dbGetAllExchangeRatesForRange(ctx context.Context, db *sqlx.DB, symbol string, interval q.Interval, start time.Time, end time.Time, ordering q.Ordering) ([]*exchangeRateModel, error) {
	res := []*exchangeRateModel{}
	err := db.SelectContext(ctx, &res,
		makeTimeBasedRangeQuery(exchangeRateTableName, "currency_code = $1 AND for_timestamp >= $2 AND for_timestamp <= $3", ordering, interval),
		symbol, start.UTC(), end.UTC(),
	)

	if err != nil {
		return nil, pgutil.CheckNoRows(err, currency.ErrNotFound)
	}
	if len(res) == 0 {
		return nil, currency.ErrNotFound
	}

	return res, nil
}

func dbGetMetadataByMint(ctx context.Context, db *sqlx.DB, mint string) (*metadataModel, error) {
	res := &metadataModel{}
	err := db.GetContext(ctx, res,
		`SELECT id, name, symbol, description, image_url, seed, authority, mint, mint_bump, decimals, currency_config, currency_config_bump, liquidity_pool, liquidity_pool_bump, vault_mint, vault_mint_bump, vault_core, vault_core_bump, fees_mint, buy_fee_bps, fees_core, sell_fee_bps, alt, created_by, created_at
		FROM `+metadataTableName+`
		WHERE mint = $1`,
		mint,
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}

func dbGetReserveByMintAndTime(ctx context.Context, db *sqlx.DB, mint string, t time.Time, ordering q.Ordering) (*reserveModel, error) {
	res := &reserveModel{}
	err := db.GetContext(ctx, res,
		makeTimeBasedGetQuery(reserveTableName, "mint = $1 AND for_date = $2 AND for_timestamp <= $3", ordering),
		mint,
		t.UTC().Format(dateFormat),
		t.UTC(),
	)
	return res, pgutil.CheckNoRows(err, currency.ErrNotFound)
}
