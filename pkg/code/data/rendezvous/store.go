package rendezvous

import (
	"context"
	"errors"
	"time"
)

var (
	ErrExists   = errors.New("rendezvous record already exists")
	ErrNotFound = errors.New("rendezvous record not found")
)

type Record struct {
	Id        uint64
	Key       string
	Address   string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Store interface {
	Put(ctx context.Context, record *Record) error

	ExtendExpiry(ctx context.Context, key, address string, expiry time.Time) error

	Delete(ctx context.Context, key, address string) error

	Get(ctx context.Context, key string) (*Record, error)
}

func (r *Record) Validate() error {
	if len(r.Key) == 0 {
		return errors.New("key is required")
	}

	if len(r.Address) == 0 {
		return errors.New("address is required")
	}

	if r.ExpiresAt.Before(time.Now()) {
		return errors.New("record is expired")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id:        r.Id,
		Key:       r.Key,
		Address:   r.Address,
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.Key = r.Key
	dst.Address = r.Address
	dst.CreatedAt = r.CreatedAt
	dst.ExpiresAt = r.ExpiresAt
}
