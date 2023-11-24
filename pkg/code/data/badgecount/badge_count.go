package badgecount

import (
	"errors"
	"time"
)

type Record struct {
	Id uint64

	Owner      string
	BadgeCount uint32

	LastUpdatedAt time.Time
	CreatedAt     time.Time
}

func (r *Record) Validate() error {
	if len(r.Owner) == 0 {
		return errors.New("owner is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Owner:      r.Owner,
		BadgeCount: r.BadgeCount,

		LastUpdatedAt: r.LastUpdatedAt,
		CreatedAt:     r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Owner = r.Owner
	dst.BadgeCount = r.BadgeCount

	dst.LastUpdatedAt = r.LastUpdatedAt
	dst.CreatedAt = r.CreatedAt
}
