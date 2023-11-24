package paymentrequest

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/pointer"
)

type Record struct {
	Id uint64

	Intent string

	DestinationTokenAccount string

	ExchangeCurrency currency.Code
	NativeAmount     float64
	ExchangeRate     *float64
	Quantity         *uint64

	Domain     *string
	IsVerified bool

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Intent) == 0 {
		return errors.New("intent id is required")
	}

	if len(r.DestinationTokenAccount) == 0 {
		return errors.New("destination token account is required")
	}

	if len(r.ExchangeCurrency) == 0 {
		return errors.New("exchange currency is required")
	}

	if r.NativeAmount == 0 {
		return errors.New("native amount cannot be zero")
	}

	if r.ExchangeRate != nil && *r.ExchangeRate == 0 {
		return errors.New("exchange rate cannot be zero when provided")
	}

	if r.Quantity != nil && *r.Quantity == 0 {
		return errors.New("quantity cannot be zero when provided")
	}

	if (r.ExchangeRate == nil) != (r.Quantity == nil) {
		return errors.New("exchange rate and quantity presence must match")
	}

	if r.Domain != nil && len(*r.Domain) == 0 {
		return errors.New("domain cannot be empty when provided")
	}

	if r.Domain == nil && r.IsVerified {
		return errors.New("cannot be verified when domain is missing")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Intent: r.Intent,

		DestinationTokenAccount: r.DestinationTokenAccount,

		ExchangeCurrency: r.ExchangeCurrency,
		NativeAmount:     r.NativeAmount,
		ExchangeRate:     pointer.Float64Copy(r.ExchangeRate),
		Quantity:         pointer.Uint64Copy(r.Quantity),

		Domain:     pointer.StringCopy(r.Domain),
		IsVerified: r.IsVerified,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Intent = r.Intent

	dst.DestinationTokenAccount = r.DestinationTokenAccount

	dst.ExchangeCurrency = r.ExchangeCurrency
	dst.NativeAmount = r.NativeAmount
	dst.ExchangeRate = pointer.Float64Copy(r.ExchangeRate)
	dst.Quantity = pointer.Uint64Copy(r.Quantity)

	dst.Domain = pointer.StringCopy(r.Domain)
	dst.IsVerified = r.IsVerified

	dst.CreatedAt = r.CreatedAt
}
