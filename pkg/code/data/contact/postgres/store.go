package postgres

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/contact"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres backed contact.Store
func New(db *sql.DB) contact.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Add implements contact.Store.Add
func (s *store) Add(ctx context.Context, owner *user.DataContainerID, contact string) error {
	model, err := newModel(owner, contact)
	if err != nil {
		return err
	}

	return model.dbAdd(ctx, s.db)
}

// BatchAdd implements contact.Store.BatchAdd
func (s *store) BatchAdd(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	return dbBatchAdd(ctx, s.db, owner, contacts)
}

// Remove implements contact.Store.Remove
func (s *store) Remove(ctx context.Context, owner *user.DataContainerID, contact string) error {
	model, err := newModel(owner, contact)
	if err != nil {
		return err
	}

	return model.dbRemove(ctx, s.db)
}

// BatchRemove implements contact.Store.BatchRemove
func (s *store) BatchRemove(ctx context.Context, owner *user.DataContainerID, contacts []string) error {
	return dbBatchRemove(ctx, s.db, owner, contacts)
}

// Get implements contact.Store.Get
func (s *store) Get(ctx context.Context, owner *user.DataContainerID, limit uint32, pageToken []byte) ([]string, []byte, error) {
	var exclusiveLowerBoundID uint64
	if len(pageToken) == 8 {
		exclusiveLowerBoundID = binary.LittleEndian.Uint64(pageToken)
	} else if len(pageToken) != 0 {
		return nil, nil, errors.New("invalid page token bytes")
	}

	models, isLastPage, err := dbGetByOwner(ctx, s.db, owner, exclusiveLowerBoundID, limit)
	if err != nil {
		return nil, nil, err
	}

	var nextPageToken []byte
	if !isLastPage {
		nextPageToken = make([]byte, 8)
		binary.LittleEndian.PutUint64(nextPageToken, uint64(models[len(models)-1].Id.Int64))
	}

	contacts := make([]string, len(models))
	for i, model := range models {
		contacts[i] = model.Contact
	}

	return contacts, nextPageToken, nil
}
