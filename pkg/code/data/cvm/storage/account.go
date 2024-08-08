package storage

import (
	"errors"
	"math"
	"time"
)

type Purpose uint8

const (
	PurposeUnknown  Purpose = iota
	PurposeDeletion         // Purely used to "delete" accounts that will never make it back to memory. At capacity, we can throw the entire account away and any DB storage.
)

type Record struct {
	Id uint64

	Vm string

	Name              string
	Address           string
	Levels            uint8
	AvailableCapacity uint64
	Purpose           Purpose

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Vm) == 0 {
		return errors.New("vm is required")
	}

	if len(r.Address) == 0 {
		return errors.New("address is required")
	}

	if len(r.Name) == 0 {
		return errors.New("name is required")
	}

	if r.Levels == 0 {
		return errors.New("levels is required")
	}

	if r.AvailableCapacity > GetMaxCapacity(r.Levels) {
		return errors.New("available capacity exceeds maximum")
	}

	if r.Purpose != PurposeDeletion {
		return errors.New("invalid purpose")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Vm: r.Vm,

		Name:              r.Name,
		Address:           r.Address,
		Levels:            r.Levels,
		AvailableCapacity: r.AvailableCapacity,
		Purpose:           r.Purpose,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Vm = r.Vm

	dst.Name = r.Name
	dst.Address = r.Address
	dst.Levels = r.Levels
	dst.AvailableCapacity = r.AvailableCapacity
	dst.Purpose = r.Purpose

	dst.CreatedAt = r.CreatedAt
}

func GetMaxCapacity(levels uint8) uint64 {
	return uint64(math.Pow(2, float64(levels))) - 1
}
