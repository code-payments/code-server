package paymentrequest

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
)

// todo: refactor model to something similar to intent
type Record struct {
	Id uint64

	Intent string

	// Payment fields
	DestinationTokenAccount *string
	ExchangeCurrency        *string
	NativeAmount            *float64
	ExchangeRate            *float64
	Quantity                *uint64

	// Login fields
	Domain     *string
	IsVerified bool

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Intent) == 0 {
		return errors.New("intent id is required")
	}

	if r.DestinationTokenAccount != nil && len(*r.DestinationTokenAccount) == 0 {
		return errors.New("destination token account is required when provided")
	}

	if r.ExchangeCurrency != nil && len(*r.ExchangeCurrency) == 0 {
		return errors.New("exchange currency is required when provided")
	}

	if r.NativeAmount != nil && *r.NativeAmount == 0 {
		return errors.New("native amount cannot be zero when provided")
	}

	if (r.DestinationTokenAccount == nil) != (r.ExchangeCurrency == nil) != (r.NativeAmount == nil) {
		return errors.New("destination token account, exchange currency and native amount presence must match")
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

	if !r.RequiresPayment() && !r.HasLogin() {
		return errors.New("request must have payment and/or login")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Intent: r.Intent,

		DestinationTokenAccount: pointer.StringCopy(r.DestinationTokenAccount),
		ExchangeCurrency:        pointer.StringCopy(r.ExchangeCurrency),
		NativeAmount:            pointer.Float64Copy(r.NativeAmount),
		ExchangeRate:            pointer.Float64Copy(r.ExchangeRate),
		Quantity:                pointer.Uint64Copy(r.Quantity),

		Domain:     pointer.StringCopy(r.Domain),
		IsVerified: r.IsVerified,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Intent = r.Intent

	dst.DestinationTokenAccount = pointer.StringCopy(r.DestinationTokenAccount)
	dst.ExchangeCurrency = pointer.StringCopy(r.ExchangeCurrency)
	dst.NativeAmount = pointer.Float64Copy(r.NativeAmount)
	dst.ExchangeRate = pointer.Float64Copy(r.ExchangeRate)
	dst.Quantity = pointer.Uint64Copy(r.Quantity)

	dst.Domain = pointer.StringCopy(r.Domain)
	dst.IsVerified = r.IsVerified

	dst.CreatedAt = r.CreatedAt
}

func (r *Record) RequiresPayment() bool {
	return len(*r.DestinationTokenAccount) > 0
}

func (r *Record) HasLogin() bool {
	return r.Domain != nil && r.IsVerified
}
