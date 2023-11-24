package rendezvous

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("rendezvous record not found")
)

type Record struct {
	Id            uint64
	Key           string
	Location      string
	CreatedAt     time.Time
	LastUpdatedAt time.Time
}

type Store interface {
	Save(ctx context.Context, record *Record) error

	Get(ctx context.Context, key string) (*Record, error)

	Delete(ctx context.Context, key string) error
}

func (r *Record) Validate() error {
	if len(r.Key) == 0 {
		return errors.New("key is required")
	}

	if len(r.Location) == 0 {
		return errors.New("location is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id:            r.Id,
		Key:           r.Key,
		Location:      r.Location,
		CreatedAt:     r.CreatedAt,
		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.Key = r.Key
	dst.Location = r.Location
	dst.CreatedAt = r.CreatedAt
	dst.LastUpdatedAt = r.LastUpdatedAt
}
