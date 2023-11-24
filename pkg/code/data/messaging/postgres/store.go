package postgres

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/messaging"
)

// todo: This doesn't support TTL expiries, which is fine for now. We can
//       manually delete old entries while in an invite-only testing phase.
type store struct {
	db *sqlx.DB
}

// New returns a postgres backed messaging.Store.
func New(db *sql.DB) messaging.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Insert implements messaging.Store.Insert.
func (s *store) Insert(ctx context.Context, record *messaging.Record) error {
	model, err := toModel(record)
	if err != nil {
		return err
	}

	return model.dbSave(ctx, s.db)
}

// Delete implements messaging.Store.Delete.
func (s *store) Delete(ctx context.Context, account string, messageID uuid.UUID) error {
	return dbDelete(ctx, s.db, account, messageID.String())
}

// Get implements messaging.Store.Get.
func (s *store) Get(ctx context.Context, account string) ([]*messaging.Record, error) {
	models, err := dbGetAllForAccount(ctx, s.db, account)
	if err != nil {
		return nil, err
	}

	records := make([]*messaging.Record, len(models))
	for i, model := range models {
		record, err := fromModel(model)
		if err != nil {
			// todo(safety): this is the equivalent QoS brick case, although should be less problematic.
			//               we could have a valve to ignore, and also to delete
			return nil, err
		}
		records[i] = record
	}

	return records, nil
}
