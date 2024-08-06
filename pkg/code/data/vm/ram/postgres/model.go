package postgres

import (
	"database/sql"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/vm/ram"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type model struct {
	Id sql.NullInt64 `db:"id"`

	Vm string `db:"vm"`

	Address string `db:"address"`

	Capacity   uint16 `db:"capacity"`
	NumSectors uint8  `db:"num_sectors"`
	NumPages   uint8  `db:"num_pages"`
	PageSize   uint8  `db:"page_size"`

	StoredAccountType uint8 `db:"stored_account_type"`

	CreatedAt time.Time `db:"created_at"`
}

func toModel(obj *ram.Record) (*model, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &model{
		Vm: obj.Vm,

		Address: obj.Address,

		Capacity:   obj.Capacity,
		NumSectors: obj.NumSectors,
		NumPages:   obj.NumPages,
		PageSize:   obj.PageSize,

		StoredAccountType: uint8(obj.StoredAccountType),

		CreatedAt: obj.CreatedAt,
	}, nil
}

func fromModel(obj *model) *ram.Record {
	return &ram.Record{
		Id: uint64(obj.Id.Int64),

		Vm: obj.Vm,

		Address: obj.Address,

		Capacity:   obj.Capacity,
		NumSectors: obj.NumSectors,
		NumPages:   obj.NumPages,
		PageSize:   obj.PageSize,

		StoredAccountType: cvm.VirtualAccountType(obj.StoredAccountType),

		CreatedAt: obj.CreatedAt,
	}
}
