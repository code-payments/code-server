package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/phone"
	"github.com/code-payments/code-server/pkg/code/data/user"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	tableName = "codewallet__core_contactlist"
)

type model struct {
	Id      sql.NullInt64 `db:"id"`
	OwnerID string        `db:"owner_id"`
	Contact string        `db:"contact"`
}

func newModel(owner *user.DataContainerID, contact string) (*model, error) {
	if err := owner.Validate(); err != nil {
		return nil, err
	}

	if !phone.IsE164Format(contact) {
		return nil, errors.New("contact phone number doesn't match E.164 standard")
	}

	return &model{
		OwnerID: owner.String(),
		// todo: Drop this eventually.
		Contact: contact,
	}, nil
}

func newModels(owner *user.DataContainerID, contacts []string) ([]*model, error) {
	models := make([]*model, len(contacts))
	for i, contact := range contacts {
		model, err := newModel(owner, contact)
		if err != nil {
			return nil, err
		}

		models[i] = model
	}

	return models, nil
}

func (m *model) dbAdd(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + tableName + `
		(owner_id, contact)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
		RETURNING id, owner_id, contact`

	err := db.QueryRowxContext(ctx, query, m.OwnerID, m.Contact).StructScan(m)
	return pgutil.CheckNoRows(err, nil)
}

func (m *model) dbRemove(ctx context.Context, db *sqlx.DB) error {
	query := `DELETE FROM ` + tableName + `
		WHERE owner_id = $1 and contact = $2
		RETURNING id, owner_id, contact`

	err := db.QueryRowxContext(ctx, query, m.OwnerID, m.Contact).StructScan(m)
	return pgutil.CheckNoRows(err, nil)
}

func dbBatchAdd(ctx context.Context, db *sqlx.DB, owner *user.DataContainerID, contacts []string) error {
	if len(contacts) == 0 {
		return nil
	}

	// Mostly doing this to reuse basic validation logic
	models, err := newModels(owner, contacts)
	if err != nil {
		return err
	}

	// todo: is there a better way to construct the query?
	query := fmt.Sprintf("INSERT INTO %s (owner_id, contact) VALUES \n", tableName)

	var entries []string
	for _, model := range models {
		entries = append(entries, fmt.Sprintf("('%s', '%s')", model.OwnerID, model.Contact))
	}

	query += strings.Join(entries, ",")
	query += "\nON CONFLICT (owner_id, contact) DO NOTHING"

	_, err = db.ExecContext(ctx, query)
	return err
}

func dbBatchRemove(ctx context.Context, db *sqlx.DB, owner *user.DataContainerID, contacts []string) error {
	if len(contacts) == 0 {
		return nil
	}

	// Mostly doing this to reuse basic validation logic
	models, err := newModels(owner, contacts)
	if err != nil {
		return err
	}

	contactArgs := make([]string, len(models))
	for i, model := range models {
		contactArgs[i] = fmt.Sprintf("'%s'", model.Contact)
	}

	// todo: is there a better way to construct the query?
	query := fmt.Sprintf(
		"DELETE FROM %s WHERE owner_id = '%s' AND contact IN (%s)\n",
		tableName,
		models[0].OwnerID,
		strings.Join(contactArgs, ","),
	)

	_, err = db.ExecContext(ctx, query)
	return err
}

func dbGetByOwner(ctx context.Context, db *sqlx.DB, owner *user.DataContainerID, exclusiveLowerBoundID uint64, limit uint32) ([]*model, bool, error) {
	var res []*model
	var isLastPage bool

	query := `
		SELECT id, owner_id, contact FROM ` + tableName + `
		WHERE owner_id = $1 AND id > $2 
		ORDER BY id ASC
		LIMIT $3
	`

	err := db.SelectContext(ctx, &res, query, owner.String(), exclusiveLowerBoundID, limit+1)
	if err != nil {
		return nil, false, err
	}

	if len(res) > int(limit) {
		res = res[:limit]
		isLastPage = false
	} else {
		isLastPage = true
	}

	return res, isLastPage, nil
}
