package postgres

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/merkletree"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres-backed merkletree.Store
func New(db *sql.DB) merkletree.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// Create implements merkletree.Store.Create
func (s *store) Create(ctx context.Context, mtdt *merkletree.Metadata) error {
	if err := merkletree.ValidateCreate(mtdt); err != nil {
		return err
	}

	model, err := toMetadataModel(mtdt)
	if err != nil {
		return err
	}

	if err := model.dbCreate(ctx, s.db); err != nil {
		return err
	}

	res := fromMetadataModel(model)
	res.CopyTo(mtdt)

	return nil
}

// GetByName implements merkletree.Store.GetByName
func (s *store) GetByName(ctx context.Context, name string) (*merkletree.Metadata, error) {
	model, err := dbGetMetadata(ctx, s.db, name)
	if err != nil {
		return nil, err
	}
	return fromMetadataModel(model), nil
}

// AddLeaf implements merkletree.Store.AddLeaf
func (s *store) AddLeaf(ctx context.Context, mtdt *merkletree.Metadata, leaf *merkletree.Node, toRoot []*merkletree.Node) error {
	if err := merkletree.ValidateAddLeaf(mtdt, leaf, toRoot); err != nil {
		return err
	}

	mtdtModel, err := toMetadataModel(mtdt)
	if err != nil {
		return err
	}

	leafModel, err := toNodeModel(leaf)
	if err != nil {
		return err
	}

	toRootModels := make([]*nodeModel, len(toRoot))
	for i, node := range toRoot {
		toRootModels[i], err = toNodeModel(node)
		if err != nil {
			return err
		}
	}

	err = dbAddLeaf(ctx, s.db, mtdtModel, leafModel, toRootModels)
	if err != nil {
		return err
	}

	updatedMtdt := fromMetadataModel(mtdtModel)
	updatedMtdt.CopyTo(mtdt)

	updatedLeaf := fromNodeModel(leafModel)
	updatedLeaf.CopyTo(leaf)

	for i, nodeModel := range toRootModels {
		updatedNode := fromNodeModel(nodeModel)
		updatedNode.CopyTo(toRoot[i])
	}

	return nil
}

// GetNode implements merkletree.Store.GetNode
func (s *store) GetNode(ctx context.Context, tree uint64, level uint8, index uint64, version uint64) (*merkletree.Node, error) {
	model, err := dbGetNode(ctx, s.db, tree, level, index, version)
	if err != nil {
		return nil, err
	}
	return fromNodeModel(model), nil
}

// GetLatestNode implements merkletree.Store.GetLatestNode
func (s *store) GetLatestNode(ctx context.Context, tree uint64, level uint8, index uint64, untilVersion uint64) (*merkletree.Node, error) {
	model, err := dbGetLatestNode(ctx, s.db, tree, level, index, untilVersion)
	if err != nil {
		return nil, err
	}
	return fromNodeModel(model), nil
}

// GetLatestNodesForProof implements merkletree.Store.GetLatestNodesForProof
func (s *store) GetLatestNodesForProof(ctx context.Context, tree uint64, levels uint8, forLeaf, untilVersion uint64) ([]*merkletree.Node, error) {
	models, err := dbGetLatestNodesForProof(ctx, s.db, tree, levels, forLeaf, untilVersion)
	if err != nil {
		return nil, err
	}

	res := make([]*merkletree.Node, len(models))
	for i, model := range models {
		res[i] = fromNodeModel(model)
	}
	return res, nil
}

// GetLatestNodesForFilledSubtrees implements merkletree.Store.GetLatestNodesForFilledSubtrees
func (s *store) GetLatestNodesForFilledSubtrees(ctx context.Context, tree uint64, levels uint8, untilVersion uint64) ([]*merkletree.Node, error) {
	models, err := dbGetLatestNodesForFilledSubtrees(ctx, s.db, tree, levels, untilVersion)
	if err != nil {
		return nil, err
	}

	res := make([]*merkletree.Node, len(models))
	for i, model := range models {
		res[i] = fromNodeModel(model)
	}
	return res, nil
}

// GetLeafByHash implements merkletree.Store.GetLeafByHash
func (s *store) GetLeafByHash(ctx context.Context, tree uint64, hash merkletree.Hash) (*merkletree.Node, error) {
	model, err := dbGetLeafByHash(ctx, s.db, tree, hash)
	if err != nil {
		return nil, err
	}
	return fromNodeModel(model), nil
}

// GetNodeByHash implements merkletree.Store.GetNodeByHash
func (s *store) GetNodeByHash(ctx context.Context, tree uint64, hash merkletree.Hash) (*merkletree.Node, error) {
	model, err := dbGetNodeByHash(ctx, s.db, tree, hash)
	if err != nil {
		return nil, err
	}
	return fromNodeModel(model), nil
}
