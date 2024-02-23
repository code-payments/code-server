package onramp

import (
	"bytes"
	"errors"
	"time"

	"github.com/google/uuid"
)

// todo: We can track provider, fulfillment state, time to fulfillment, etc.
type Record struct {
	Id uint64

	Owner    string
	Currency string
	Amount   float64
	Nonce    uuid.UUID

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Owner) == 0 {
		return errors.New("owner is required")
	}

	if len(r.Currency) == 0 {
		return errors.New("currency is required")
	}

	if r.Amount <= 0 {
		return errors.New("amount must be positive")
	}

	if bytes.Equal(r.Nonce[:], uuid.Nil[:]) {
		return errors.New("nonce is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Owner:    r.Owner,
		Currency: r.Currency,
		Amount:   r.Amount,
		Nonce:    r.Nonce,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Owner = r.Owner
	dst.Currency = r.Currency
	dst.Amount = r.Amount
	dst.Nonce = r.Nonce

	dst.CreatedAt = r.CreatedAt
}
