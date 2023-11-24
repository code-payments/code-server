package paywall

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/currency"
)

type Record struct {
	Id uint64

	OwnerAccount            string
	DestinationTokenAccount string

	ExchangeCurrency currency.Code
	NativeAmount     float64
	RedirectUrl      string
	ShortPath        string

	Signature string

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.OwnerAccount) == 0 {
		return errors.New("owner account is required")
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

	if len(r.RedirectUrl) == 0 {
		return errors.New("redirect url is required")
	}

	if len(r.ShortPath) == 0 {
		return errors.New("short path is required")
	}

	if len(r.Signature) == 0 {
		return errors.New("signature is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		OwnerAccount:            r.OwnerAccount,
		DestinationTokenAccount: r.DestinationTokenAccount,

		ExchangeCurrency: r.ExchangeCurrency,
		NativeAmount:     r.NativeAmount,
		RedirectUrl:      r.RedirectUrl,
		ShortPath:        r.ShortPath,

		Signature: r.Signature,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.OwnerAccount = r.OwnerAccount
	dst.DestinationTokenAccount = r.DestinationTokenAccount

	dst.ExchangeCurrency = r.ExchangeCurrency
	dst.NativeAmount = r.NativeAmount
	dst.RedirectUrl = r.RedirectUrl
	dst.ShortPath = r.ShortPath

	dst.Signature = r.Signature

	dst.CreatedAt = r.CreatedAt
}
