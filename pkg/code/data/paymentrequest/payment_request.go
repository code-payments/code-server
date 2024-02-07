package paymentrequest

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
)

// Currently, the model supports payment, login, or both.
//
// todo: Refactor to a structure similar to intent as we add more request use cases.
type Record struct {
	Id uint64

	Intent string

	// Payment fields
	DestinationTokenAccount *string
	ExchangeCurrency        *string
	NativeAmount            *float64
	ExchangeRate            *float64
	Quantity                *uint64
	Fees                    []*Fee

	// Login fields
	Domain     *string
	IsVerified bool

	CreatedAt time.Time
}

type Fee struct {
	DestinationTokenAccount string
	// todo: how are we representing the amount?
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

	if (r.DestinationTokenAccount == nil) != (r.NativeAmount == nil) {
		return errors.New("destination token account and native amount presence must match")
	}

	if (r.NativeAmount == nil) != (r.ExchangeCurrency == nil) {
		return errors.New("exchange currency and native amount presence must match")
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

	if len(r.Fees) > 0 && r.DestinationTokenAccount == nil {
		return errors.New("fees cannot be present without payment")
	}

	for _, fee := range r.Fees {
		if err := fee.Validate(); err != nil {
			return err
		}
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
	fees := make([]*Fee, len(r.Fees))
	for i, fee := range r.Fees {
		copied := fee.Clone()
		fees[i] = &copied
	}

	return Record{
		Id: r.Id,

		Intent: r.Intent,

		DestinationTokenAccount: pointer.StringCopy(r.DestinationTokenAccount),
		ExchangeCurrency:        pointer.StringCopy(r.ExchangeCurrency),
		NativeAmount:            pointer.Float64Copy(r.NativeAmount),
		ExchangeRate:            pointer.Float64Copy(r.ExchangeRate),
		Quantity:                pointer.Uint64Copy(r.Quantity),
		Fees:                    fees,

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

	dst.Fees = make([]*Fee, len(r.Fees))
	for i, fee := range r.Fees {
		copied := fee.Clone()
		dst.Fees[i] = &copied
	}
}

func (r *Record) RequiresPayment() bool {
	return r.DestinationTokenAccount != nil
}

func (r *Record) HasLogin() bool {
	return r.Domain != nil && r.IsVerified
}

func (f *Fee) Validate() error {
	if len(f.DestinationTokenAccount) == 0 {
		return errors.New("fee destination token account is required")
	}

	return nil
}

func (f *Fee) Clone() Fee {
	return Fee{
		DestinationTokenAccount: f.DestinationTokenAccount,
	}
}

func (f *Fee) CopyTo(dst *Fee) {
	dst.DestinationTokenAccount = f.DestinationTokenAccount
}
