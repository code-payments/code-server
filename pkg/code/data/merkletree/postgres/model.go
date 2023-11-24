package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	pg "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

const (
	metadataTableName = "codewallet__core_merkletreemetadata"
	nodeTableName     = "codewallet__core_merkletreenodes"
)

type metadataModel struct {
	Id        sql.NullInt64 `db:"id"`
	Name      string        `db:"name"`
	Levels    int           `db:"levels"`
	NextIndex uint64        `db:"next_index"`
	Seed      []byte        `db:"seed"`
	CreatedAt time.Time     `db:"created_at"`
}

type nodeModel struct {
	Id        sql.NullInt64 `db:"id"`
	TreeId    int64         `db:"tree_id"`
	Level     int           `db:"level"`
	Index     uint64        `db:"index"`
	Hash      []byte        `db:"hash"`
	LeafValue []byte        `db:"leaf_value"`
	Version   uint64        `db:"version"`
	CreatedAt time.Time     `db:"created_at"`
}

func toMetadataModel(data *merkletree.Metadata) (*metadataModel, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	seedCopy := make([]byte, len(data.Seed))
	copy(seedCopy, data.Seed)

	return &metadataModel{
		Name:      data.Name,
		Levels:    int(data.Levels),
		NextIndex: data.NextIndex,
		Seed:      seedCopy,
		CreatedAt: data.CreatedAt,
	}, nil
}

func fromMetadataModel(model *metadataModel) *merkletree.Metadata {
	seedCopy := make([]byte, len(model.Seed))
	copy(seedCopy, model.Seed)

	return &merkletree.Metadata{
		Id:        uint64(model.Id.Int64),
		Name:      model.Name,
		Levels:    uint8(model.Levels),
		NextIndex: model.NextIndex,
		Seed:      seedCopy,
		CreatedAt: model.CreatedAt,
	}
}

func toNodeModel(data *merkletree.Node) (*nodeModel, error) {
	if err := data.Validate(); err != nil {
		return nil, err
	}

	return &nodeModel{
		TreeId:    int64(data.TreeId),
		Level:     int(data.Level),
		Index:     data.Index,
		Hash:      data.Hash,
		LeafValue: data.LeafValue,
		Version:   data.Version,
		CreatedAt: data.CreatedAt,
	}, nil
}

func fromNodeModel(model *nodeModel) *merkletree.Node {
	return &merkletree.Node{
		Id:        uint64(model.Id.Int64),
		TreeId:    uint64(model.TreeId),
		Level:     uint8(model.Level),
		Index:     model.Index,
		Hash:      model.Hash,
		LeafValue: model.LeafValue,
		Version:   model.Version,
		CreatedAt: model.CreatedAt,
	}
}

func (m *metadataModel) dbCreate(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + metadataTableName + `
		(name, levels, next_index, seed, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, levels, next_index, seed, created_at
	`

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	err := db.QueryRowxContext(
		ctx,
		query,
		m.Name,
		m.Levels,
		m.NextIndex,
		m.Seed,
		m.CreatedAt,
	).StructScan(m)

	if err != nil {
		return pg.CheckUniqueViolation(err, merkletree.ErrMetadataExists)
	}
	return nil
}

func (m *metadataModel) dbUpdateInTx(ctx context.Context, tx *sqlx.Tx) error {
	query := `UPDATE ` + metadataTableName + `
		SET next_index = $2
		WHERE name = $1 AND next_index = $2 - 1
		RETURNING id, name, levels, next_index, seed, created_at
	`
	err := tx.QueryRowxContext(ctx, query, m.Name, m.NextIndex).StructScan(m)
	if err != nil {
		return pg.CheckNoRows(err, merkletree.ErrInvalidMetadata)
	}
	return nil
}

func (m *nodeModel) dbCreateInTx(ctx context.Context, tx *sqlx.Tx) error {
	query := `INSERT INTO ` + nodeTableName + `
		(tree_id, level, index, hash, leaf_value, version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, tree_id, level, index, hash, leaf_value, version, created_at
	`

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	err := tx.QueryRowxContext(
		ctx,
		query,
		m.TreeId,
		m.Level,
		m.Index,
		m.Hash,
		m.LeafValue,
		m.Version,
		m.CreatedAt,
	).StructScan(m)
	if err != nil {
		return pg.CheckUniqueViolation(err, merkletree.ErrNodeExists)
	}

	return nil
}

func dbGetMetadata(ctx context.Context, db *sqlx.DB, name string) (*metadataModel, error) {
	var res metadataModel

	query := `SELECT id, name, levels, next_index, seed, created_at FROM ` + metadataTableName + `
		WHERE name = $1
	`
	err := db.GetContext(ctx, &res, query, name)
	if err != nil {
		return nil, pg.CheckNoRows(err, merkletree.ErrMetadataNotFound)
	}

	return &res, nil
}

func dbAddLeaf(ctx context.Context, db *sqlx.DB, mtdt *metadataModel, leaf *nodeModel, toRoot []*nodeModel) error {
	return pg.ExecuteInTx(ctx, db, sql.LevelDefault, func(tx *sqlx.Tx) error {
		err := mtdt.dbUpdateInTx(ctx, tx)
		if err != nil {
			return err
		}

		err = leaf.dbCreateInTx(ctx, tx)
		if err != nil {
			return err
		}

		for _, node := range toRoot {
			err = node.dbCreateInTx(ctx, tx)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func dbGetNode(ctx context.Context, db *sqlx.DB, tree uint64, level uint8, index uint64, version uint64) (*nodeModel, error) {
	var res nodeModel

	query := `SELECT id, tree_id, level, index, hash, leaf_value, version, created_at  FROM ` + nodeTableName + `
		WHERE tree_id = $1 AND level = $2 AND index = $3 AND version = $4
	`

	err := db.GetContext(ctx, &res, query, tree, level, index, version)
	if err != nil {
		return nil, pg.CheckNoRows(err, merkletree.ErrNodeNotFound)
	}
	return &res, nil
}

func dbGetLatestNode(ctx context.Context, db *sqlx.DB, tree uint64, level uint8, index uint64, untilVersion uint64) (*nodeModel, error) {
	var res nodeModel

	query := `SELECT id, tree_id, level, index, hash, leaf_value, version, created_at FROM ` + nodeTableName + `
		WHERE tree_id = $1 AND level = $2 AND index = $3 AND version <= $4
		ORDER BY version DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, &res, query, tree, level, index, untilVersion)
	if err != nil {
		return nil, pg.CheckNoRows(err, merkletree.ErrNodeNotFound)
	}
	return &res, nil
}

func dbGetLatestNodesForProof(ctx context.Context, db *sqlx.DB, tree uint64, levels uint8, forLeaf, untilVersion uint64) ([]*nodeModel, error) {
	var query string

	var res []*nodeModel
	currentLevelIndex := forLeaf
	for level := uint8(0); level < levels; level++ {
		var siblingIndex uint64
		if currentLevelIndex%2 == 0 {
			siblingIndex = currentLevelIndex + 1
		} else {
			siblingIndex = currentLevelIndex - 1
		}

		// This is the most naive approach to a query, but it saves us from making
		// 63 DB calls over the network serially.

		if len(query) != 0 {
			query += "UNION ALL\n"
		}

		query += fmt.Sprintf(`(SELECT id, tree_id, level, index, hash, leaf_value, version, created_at FROM `+nodeTableName+`
			WHERE tree_id = %d AND level = %d AND index = %d AND version <= %d
			ORDER BY version DESC
			LIMIT 1)
		`, tree, level, siblingIndex, untilVersion)

		currentLevelIndex = currentLevelIndex / 2
	}

	err := db.SelectContext(ctx, &res, query)
	if err != nil {
		return nil, pg.CheckNoRows(err, nil)
	}
	return res, nil
}

func dbGetLatestNodesForFilledSubtrees(ctx context.Context, db *sqlx.DB, tree uint64, levels uint8, untilVersion uint64) ([]*nodeModel, error) {
	var query string

	var res []*nodeModel
	currentLevelIndex := untilVersion - 1
	for level := uint8(0); level < levels; level++ {
		nodeIndex := currentLevelIndex
		if nodeIndex%2 != 0 {
			nodeIndex = nodeIndex - 1
		}

		// This is the most naive approach to a query, but it saves us from making
		// 63 DB calls over the network serially.

		if len(query) != 0 {
			query += "UNION\n"
		}

		query += fmt.Sprintf(`(SELECT id, tree_id, level, index, hash, leaf_value, version, created_at FROM `+nodeTableName+`
			WHERE tree_id = %d AND level = %d AND index = %d AND version <= %d
			ORDER BY version DESC
			LIMIT 1)
		`, tree, level, nodeIndex, untilVersion)

		currentLevelIndex = currentLevelIndex / 2
	}

	err := db.SelectContext(ctx, &res, query)
	if err != nil {
		return nil, pg.CheckNoRows(err, nil)
	}
	return res, nil
}

func dbGetLeafByHash(ctx context.Context, db *sqlx.DB, tree uint64, hash merkletree.Hash) (*nodeModel, error) {
	var res nodeModel

	query := `SELECT id, tree_id, level, index, hash, leaf_value, version, created_at FROM ` + nodeTableName + `
		WHERE tree_id = $1 AND hash = $2 AND level = 0
		ORDER BY version DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, &res, query, tree, hash)
	if err != nil {
		return nil, pg.CheckNoRows(err, merkletree.ErrNodeNotFound)
	}
	return &res, nil
}

func dbGetNodeByHash(ctx context.Context, db *sqlx.DB, tree uint64, hash merkletree.Hash) (*nodeModel, error) {
	var res nodeModel

	query := `SELECT id, tree_id, level, index, hash, leaf_value, version, created_at FROM ` + nodeTableName + `
		WHERE tree_id = $1 AND hash = $2
		ORDER BY version DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, &res, query, tree, hash)
	if err != nil {
		return nil, pg.CheckNoRows(err, merkletree.ErrNodeNotFound)
	}
	return &res, nil
}
