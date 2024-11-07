package ram

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type Record struct {
	Id uint64

	Vm string

	Address string

	Capacity   uint16
	NumSectors uint16
	NumPages   uint16
	PageSize   uint8

	StoredAccountType cvm.VirtualAccountType

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Vm) == 0 {
		return errors.New("vm is required")
	}

	if len(r.Address) == 0 {
		return errors.New("address is required")
	}

	if r.Capacity == 0 {
		return errors.New("capacity is required")
	}

	if r.NumSectors == 0 {
		return errors.New("sector count is required")
	}

	if r.NumPages == 0 {
		return errors.New("pages count is required")
	}

	switch r.StoredAccountType {
	case cvm.VirtualAccountTypeDurableNonce, cvm.VirtualAccountTypeRelay, cvm.VirtualAccountTypeTimelock:
	default:
		return errors.New("invalid stored account type")
	}

	if r.PageSize == 0 {
		return errors.New("page size is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Vm: r.Vm,

		Address: r.Address,

		Capacity:   r.Capacity,
		NumSectors: r.NumSectors,
		NumPages:   r.NumPages,
		PageSize:   r.PageSize,

		StoredAccountType: r.StoredAccountType,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Vm = r.Vm

	dst.Address = r.Address

	dst.Capacity = r.Capacity
	dst.NumSectors = r.NumSectors
	dst.NumPages = r.NumPages
	dst.PageSize = r.PageSize

	dst.StoredAccountType = r.StoredAccountType

	dst.CreatedAt = r.CreatedAt
}
