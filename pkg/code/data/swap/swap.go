package swap

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
)

type State uint8

// todo: define all states
const (
	StateUnknown State = iota
	StateCreated
	StateFunding
	StateFunded
	StateSubmitting
	StateFinalized
	StateFailed
	StateCancelling
	StateCancelled
)

type FundingSource uint8

const (
	FundingSourceUnknown = iota
	FundingSourceSubmitIntent
)

type Record struct {
	Id uint64

	SwapId string

	Owner string

	FromMint string
	ToMint   string
	Amount   uint64

	FundingId     string
	FundingSource FundingSource

	Nonce     string
	Blockhash string

	ProofSignature string

	TransactionSignature *string
	TransactionBlob      []byte

	State State

	Version uint64

	CreatedAt time.Time
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		SwapId: r.SwapId,

		Owner: r.Owner,

		FromMint: r.FromMint,
		ToMint:   r.ToMint,
		Amount:   r.Amount,

		FundingId:     r.FundingId,
		FundingSource: r.FundingSource,

		Nonce:     r.Nonce,
		Blockhash: r.Blockhash,

		ProofSignature: r.ProofSignature,

		TransactionSignature: pointer.StringCopy(r.TransactionSignature),
		TransactionBlob:      r.TransactionBlob,

		State: r.State,

		Version: r.Version,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.SwapId = r.SwapId

	dst.Owner = r.Owner

	dst.FromMint = r.FromMint
	dst.ToMint = r.ToMint
	dst.Amount = r.Amount

	dst.FundingId = r.FundingId
	dst.FundingSource = r.FundingSource

	dst.Nonce = r.Nonce
	dst.Blockhash = r.Blockhash

	dst.ProofSignature = r.ProofSignature

	dst.TransactionSignature = pointer.StringCopy(r.TransactionSignature)
	dst.TransactionBlob = r.TransactionBlob

	dst.State = r.State

	dst.Version = r.Version

	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.SwapId) == 0 {
		return errors.New("swap id is requried")
	}

	if len(r.Owner) == 0 {
		return errors.New("owner is required")
	}

	if len(r.FromMint) == 0 {
		return errors.New("source mint is required")
	}

	if len(r.ToMint) == 0 {
		return errors.New("destination mint is required")
	}

	if r.Amount == 0 {
		return errors.New("amount is required")
	}

	if len(r.FundingId) == 0 {
		return errors.New("funding id is required")
	}

	if r.FundingSource == FundingSourceUnknown {
		return errors.New("funding source is required")
	}

	if len(r.Nonce) == 0 {
		return errors.New("nonce is required")
	}

	if len(r.Blockhash) == 0 {
		return errors.New("blockhash is required")
	}

	if len(r.ProofSignature) == 0 {
		return errors.New("proof signature is required")
	}

	if r.TransactionSignature != nil && len(*r.TransactionSignature) == 0 {
		return errors.New("transaction signature is empty")
	}

	if len(r.TransactionBlob) != 0 && r.TransactionSignature == nil {
		return errors.New("transaction signature is missing")
	}

	return nil
}

func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateFunding:
		return "funding"
	case StateFunded:
		return "funded"
	case StateSubmitting:
		return "submitting"
	case StateFinalized:
		return "finalized"
	case StateFailed:
		return "failed"
	case StateCancelling:
		return "cancelling"
	case StateCancelled:
		return "cancelled"
	}
	return "unknown"
}
