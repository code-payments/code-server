package balance

import (
	"errors"
	"time"
)

// Note: Only supports external balances
type Record struct {
	Id uint64

	TokenAccount   string
	Quarks         uint64
	SlotCheckpoint uint64

	LastUpdatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.TokenAccount) == 0 {
		return errors.New("token account is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		TokenAccount:   r.TokenAccount,
		Quarks:         r.Quarks,
		SlotCheckpoint: r.SlotCheckpoint,

		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.TokenAccount = r.TokenAccount
	dst.Quarks = r.Quarks
	dst.SlotCheckpoint = r.SlotCheckpoint

	dst.LastUpdatedAt = r.LastUpdatedAt
}
