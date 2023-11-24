package kikcode

import (
	"crypto/ed25519"
	"crypto/rand"
	"slices"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kikcode/encoding"
)

/*

Layout 0: Cash

   0   1   2   3   4   5   6   7   8   9  10  11  12  13  14  15  16  17  18  19
 +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+
 | T |            Amount             |                   Nonce                   |
 +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+

 (T) Type (1 byte)

 The first byte of the data in all Code scan codes is reserved for the scan
 code type. This field indicates which type of scan code data is contained
 in the scan code. The expected format for each type is outlined below.

 Kin Amount in Quarks (8 bytes)

 This field indicates the number of quarks the payment is for. It should be
 represented as a 64-bit unsigned integer.

 Nonce (11 bytes)

 This field is an 11-byte randomly-generated nonce. It should be regenerated
 each time a new payment is initiated.

 Layout 1: Gift Card

 Same as layout 0.

 Layout 2: Payment Request

   0   1   2   3   4   5   6   7   8   9  10  11  12  13  14  15  16  17  18  19
 +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+
 | T | C |        Fiat               |                   Nonce                   |
 +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+

 (T) Type (1 byte)

 The first byte of the data in all Code scan codes is reserved for the scan
 code type. This field indicates which type of scan code data is contained
 in the scan code. The expected format for each type is outlined below.

 (C) Currency Code (1 bytes)

 This field indicates the currency code for the fiat amount. The value is an
 encoded index less than 255 that maps to a currency code in CurrencyCode.swift

 Fiat Amount (7 bytes)

 This field indicates the fiat amount the payment is for. It should represent the
 value multiplied by 100 in a 7 byte buffer.

 Nonce (11 bytes)

 This field is an 11-byte randomly-generated nonce. It should be regenerated
 each time a new payment is initiated.

*/

type Kind uint8

const (
	Cash Kind = iota
	GiftCard
	PaymentRequest
)

const (
	typeSize    = 1
	amountSize  = 8
	nonceSize   = 11
	payloadSize = 20
)

type IdempotencyKey [nonceSize]byte

type Payload struct {
	kind         Kind
	amountBuffer amountBuffer
	nonce        IdempotencyKey
}

func NewPayloadFromKinAmount(kind Kind, quarks uint64, nonce IdempotencyKey) *Payload {
	return &Payload{
		kind:         kind,
		amountBuffer: newKinAmountBuffer(quarks),
		nonce:        nonce,
	}
}

func NewPayloadFromFiatAmount(kind Kind, currency currency.Code, amount float64, nonce IdempotencyKey) (*Payload, error) {
	amountBuffer, err := newFiatAmountBuffer(currency, amount)
	if err != nil {
		return nil, err
	}

	return &Payload{
		kind:         kind,
		amountBuffer: amountBuffer,
		nonce:        nonce,
	}, nil
}

func (p *Payload) ToBytes() []byte {
	var buffer [payloadSize]byte
	buffer[0] = byte(p.kind)

	amountBuffer := p.amountBuffer.ToBytes()
	for i := 0; i < amountSize; i++ {
		buffer[i+typeSize] = amountBuffer[i]
	}

	for i := 0; i < nonceSize; i++ {
		buffer[i+typeSize+amountSize] = p.nonce[i]
	}

	return buffer[:]
}

func (p *Payload) ToQrCodeDescription(dimension float64) (*Description, error) {
	viewPayload, err := encoding.Encode(p.ToBytes())
	if err != nil {
		return nil, err
	}

	kikCodePayload := CreateKikCodePayload(viewPayload)

	return GenerateDescription(dimension, kikCodePayload)
}

func (p *Payload) GetIdempotencyKey() IdempotencyKey {
	return p.nonce
}

func (p *Payload) ToRendezvousKey() ed25519.PrivateKey {
	return DeriveRendezvousPrivateKey(p)
}

func GenerateRandomIdempotencyKey() IdempotencyKey {
	var buffer [nonceSize]byte
	rand.Read(buffer[:])
	return buffer
}

type amountBuffer interface {
	ToBytes() [amountSize]byte
}

type kinAmountBuffer struct {
	quarks uint64
}

func newKinAmountBuffer(quarks uint64) amountBuffer {
	return &kinAmountBuffer{
		quarks: quarks,
	}
}

func (b *kinAmountBuffer) ToBytes() [amountSize]byte {
	var buffer [amountSize]byte
	for i := 0; i < amountSize; i++ {
		buffer[i] = byte(b.quarks >> uint64(8*i) & uint64(0xFF))
	}
	return buffer
}

type fiatAmountBuffer struct {
	currency currency.Code
	amount   float64
}

func newFiatAmountBuffer(currency currency.Code, amount float64) (amountBuffer, error) {
	index := slices.Index(supportedCurrenies, currency)
	if index < 0 {
		return nil, errors.Errorf("%s currency is not supported", currency)
	}

	return &fiatAmountBuffer{
		currency: currency,
		amount:   amount,
	}, nil
}

func (b *fiatAmountBuffer) ToBytes() [amountSize]byte {
	var buffer [amountSize]byte

	buffer[0] = byte(slices.Index(supportedCurrenies, b.currency))

	amountToSerialize := uint64(b.amount * 100)
	for i := 1; i < amountSize; i++ {
		buffer[i] = byte(amountToSerialize >> uint64(8*(i-1)) & uint64(0xFF))
	}

	return buffer
}
