package deposit

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

var (
	ErrDepositNotFound = errors.New("external deposit not found")
)

// Note: Only captures external deposits at the transaction level
type Record struct {
	Id uint64

	Signature      string
	Destination    string
	Amount         uint64
	UsdMarketValue float64

	Slot              uint64
	ConfirmationState transaction.Confirmation

	CreatedAt time.Time
}

type Store interface {
	// Save saves a deposit record
	Save(ctx context.Context, record *Record) error

	// Get gets a deposit record for a signature and account
	Get(ctx context.Context, signature, account string) (*Record, error)

	// GetQuarkAmount gets the total deposited quark amount to an account
	// for finalized transactions
	GetQuarkAmount(ctx context.Context, account string) (uint64, error)

	// GetQuarkAmountBatch is like GetQuarkAmount but for a batch of accounts
	GetQuarkAmountBatch(ctx context.Context, accounts ...string) (map[string]uint64, error)

	// GetUsdAmount gets the total deposited USD amount to an account for finalized
	// transactions
	GetUsdAmount(ctx context.Context, account string) (float64, error)
}

func (r *Record) Validate() error {
	if len(r.Signature) == 0 {
		return errors.New("signature is required")
	}

	if len(r.Destination) == 0 {
		return errors.New("destination is required")
	}

	if r.Amount == 0 {
		return errors.New("amount is required")
	}

	if r.UsdMarketValue <= 0 {
		return errors.New("usd market value must be positive")
	}

	if r.ConfirmationState == transaction.ConfirmationUnknown {
		return errors.New("confirmation state is required")
	}

	if r.ConfirmationState == transaction.ConfirmationFinalized && r.Slot == 0 {
		return errors.New("slot is required for finalized deposits")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Signature:      r.Signature,
		Destination:    r.Destination,
		Amount:         r.Amount,
		UsdMarketValue: r.UsdMarketValue,

		Slot:              r.Slot,
		ConfirmationState: r.ConfirmationState,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Signature = r.Signature
	dst.Destination = r.Destination
	dst.Amount = r.Amount
	dst.UsdMarketValue = r.UsdMarketValue

	dst.Slot = r.Slot
	dst.ConfirmationState = r.ConfirmationState

	dst.CreatedAt = r.CreatedAt
}
