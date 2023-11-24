package payment

import (
	"bytes"
	"time"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana/token"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
)

type ByBlock []*Record

func (a ByBlock) Len() int           { return len(a) }
func (a ByBlock) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByBlock) Less(i, j int) bool { return a[i].BlockId < a[j].BlockId }

// The structure for metadata behind a payment or token transfer between two
// parties. This data is considered untrusted as it comes from the client apps
// directly and not from the blockchain. It gives us the intended native
// currencies for the agreed upon exchange between two app users. This data
// cannot be derived from the blockchain alone. We could guess at it, but then
// we would definitely be off by a couple decimal points every now and then when
// reporting the booking cost back to the user.
//
// Note: This is generally unused right now and should be deprecated with the
// new intent system and external data models. There's a few use cases still
// hitting this which, in particular, need to know the order of transfers.
type Record struct {
	Id uint64 // The internal database id for this transaction

	BlockId          uint64
	BlockTime        time.Time
	TransactionId    string // The signature of the Solana transaction, which could contain multiple payments
	TransactionIndex uint32 // The index that the transfer (payment) instruction appears at inside the Solana transaction
	Rendezvous       string // The public key of the party that is the rendezvous point for this payment (might be empty)
	IsExternal       bool   // External payments are deprecated, in favour of the new deposit store

	Source      string // The source account id for this payment
	Destination string // The destination account id for this payment
	Quantity    uint64 // The amount of Kin (in Quarks)

	ExchangeCurrency string  // The (external) agreed upon currency for the exchange
	ExchangeRate     float64 // The (external) agreed upon exchange rate for determining the amount of Kin to transfer
	UsdMarketValue   float64 // The (internal) market value of this transfer based on the internal exchange rate record
	Region           *string // The (external) agreed upon country flag for the currency

	IsWithdraw bool

	ConfirmationState transaction.Confirmation
	CreatedAt         time.Time
}

type PaymentType uint32

const (
	PaymentType_Send PaymentType = iota
	PaymentType_Receive
)

func NewFromTransfer(transfer *token.DecompiledTransfer, sig string, index int, rate float64, now time.Time) *Record {
	source_id := base58.Encode(transfer.Source)
	destination_id := base58.Encode(transfer.Destination)

	return &Record{
		TransactionId:     sig,
		TransactionIndex:  uint32(index),
		Source:            source_id,
		Destination:       destination_id,
		Quantity:          transfer.Amount,
		ExchangeCurrency:  "kin",
		ExchangeRate:      1,
		UsdMarketValue:    rate * float64(kin.FromQuarks(transfer.Amount)),
		ConfirmationState: transaction.ConfirmationPending,
		CreatedAt:         now,
	}
}

func NewFromTransferChecked(transfer *token.DecompiledTransfer2, sig string, index int, rate float64, now time.Time) (*Record, error) {
	if !bytes.Equal(transfer.Mint, kin.TokenMint) {
		return nil, errors.New("invalid token mint")
	}

	if transfer.Decimals != kin.Decimals {
		return nil, errors.New("invalid kin token decimals")
	}

	source_id := base58.Encode(transfer.Source)
	destination_id := base58.Encode(transfer.Destination)

	return &Record{
		TransactionId:     sig,
		TransactionIndex:  uint32(index),
		Source:            source_id,
		Destination:       destination_id,
		Quantity:          transfer.Amount,
		ExchangeCurrency:  "kin",
		ExchangeRate:      1,
		UsdMarketValue:    rate * float64(kin.FromQuarks(transfer.Amount)),
		ConfirmationState: transaction.ConfirmationPending,
		CreatedAt:         now,
	}, nil
}
