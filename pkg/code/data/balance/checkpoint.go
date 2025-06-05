package balance

import (
	"errors"
	"time"
)

type ExternalCheckpointRecord struct {
	Id uint64

	TokenAccount   string
	Quarks         uint64
	SlotCheckpoint uint64

	LastUpdatedAt time.Time
}

func (r *ExternalCheckpointRecord) Validate() error {
	if len(r.TokenAccount) == 0 {
		return errors.New("token account is required")
	}

	return nil
}

func (r *ExternalCheckpointRecord) Clone() ExternalCheckpointRecord {
	return ExternalCheckpointRecord{
		Id: r.Id,

		TokenAccount:   r.TokenAccount,
		Quarks:         r.Quarks,
		SlotCheckpoint: r.SlotCheckpoint,

		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *ExternalCheckpointRecord) CopyTo(dst *ExternalCheckpointRecord) {
	dst.Id = r.Id

	dst.TokenAccount = r.TokenAccount
	dst.Quarks = r.Quarks
	dst.SlotCheckpoint = r.SlotCheckpoint

	dst.LastUpdatedAt = r.LastUpdatedAt
}
