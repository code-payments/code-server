package kin

import (
	"encoding/base64"

	"github.com/pkg/errors"
)

// todo: formalize
const magicByte = 0x1

// TransactionType is the memo transaction type.
type TransactionType int16

const (
	TransactionTypeUnknown TransactionType = iota - 1
	TransactionTypeNone
	TransactionTypeEarn
	TransactionTypeSpend
	TransactionTypeP2P
)

// HighestVersion is the highest 'supported' memo version by
// the implementation.
const HighestVersion = 1

// MaxTransactionType is the maximum transaction type 'supported'
// by the implementation.
const MaxTransactionType = TransactionTypeP2P

// Memo is the 32 byte memo encoded into transactions, as defined
// in github.com/kinecosystem/agora-api.
type Memo [32]byte

// NewMemo creates a new Memo with the specified parameters.
func NewMemo(v byte, t TransactionType, appIndex uint16, foreignKey []byte) (m Memo, err error) {
	if len(foreignKey) > 29 {
		return m, errors.Errorf("invalid foreign key length: %d", len(foreignKey))
	}
	if v > 7 {
		return m, errors.Errorf("invalid version")
	}

	if t < 0 || t > 31 {
		return m, errors.Errorf("invalid transaction type")
	}

	m[0] = magicByte
	m[0] |= v << 2
	m[0] |= (byte(t) & 0x7) << 5

	m[1] = (byte(t) & 0x18) >> 3
	m[1] |= byte(appIndex&0x3f) << 2

	m[2] = byte((appIndex & 0x3fc0) >> 6)

	m[3] = byte((appIndex & 0xc000) >> 14)

	if len(foreignKey) > 0 {
		m[3] |= (foreignKey[0] & 0x3f) << 2

		// insert the rest of the fk. since each loop references fk[n] and fk[n+1], the upper bound is offset by 3 instead of 4.
		for i := 4; i < 3+len(foreignKey); i++ {
			// apply last 2-bits of current byte
			// apply first 6-bits of next byte
			m[i] = (foreignKey[i-4] >> 6) & 0x3
			m[i] |= (foreignKey[i-3] & 0x3f) << 2
		}

		// if the foreign key is less than 29 bytes, the last 2 bits of the FK can be included in the memo
		if len(foreignKey) < 29 {
			m[len(foreignKey)+3] = (foreignKey[len(foreignKey)-1] >> 6) & 0x3
		}
	}

	return m, nil
}

func MemoFromBase64String(b64 string, strict bool) (m Memo, err error) {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return m, errors.Wrap(err, "invalid b64")
	}

	copy(m[:], b)
	if strict {
		if ok := IsValidMemoStrict(m); !ok {
			return m, errors.New("not a valid memo")
		}
		return m, nil
	}

	if ok := IsValidMemo(m); !ok {
		return m, errors.New("not a valid memo")
	}

	return m, nil
}

// IsValidMemo returns whether or not the memo is valid.
//
// It should be noted that there are no guarantees if the
// memo is valid, only if the memo is invalid. That is, this
// function may return false positives.
//
// Stricter validation can be done via the IsValidMemoStrict.
// However, IsValidMemoStrict is not as forward compatible as
// IsValidMemo.
func IsValidMemo(m Memo) bool {
	if m[0]&0x3 != magicByte {
		return false
	}

	return m.TransactionTypeRaw() != TransactionTypeUnknown
}

// IsValidMemoStrict returns whether or not the memo is valid
// checking against this SDKs supported version and transaction types.
//
// It should be noted that there are no guarantees if the
// memo is valid, only if the memo is invalid. That is, this
// function may return false positives.
func IsValidMemoStrict(m Memo) bool {
	if !IsValidMemo(m) {
		return false
	}

	if m.Version() > HighestVersion {
		return false
	}
	if m.TransactionType() == TransactionTypeUnknown {
		return false
	}

	return true
}

// Version returns the version of the memo.
func (m Memo) Version() byte {
	return (m[0] & 0x1c) >> 2
}

// TransactionType returns the transaction type of the memo.
func (m Memo) TransactionType() TransactionType {
	if t := m.TransactionTypeRaw(); t <= MaxTransactionType {
		return t
	}

	return TransactionTypeUnknown
}

// TransactionTypeRaw returns the transaction type of the memo,
// even if it is unsupported by this SDK. It should only be used
// as a fall back if the raw value is needed when TransactionType()
// yields TransactionTypeUnknown.
func (m Memo) TransactionTypeRaw() TransactionType {
	// Note: we intentionally don't wrap around to Unknown
	// for debugging and potential future compat.
	return TransactionType((m[0] >> 5) | (m[1]&0x3)<<3)
}

// AppIndex returns the app index of the memo.
func (m Memo) AppIndex() uint16 {
	a := uint16(m[1]) >> 2
	b := uint16(m[2]) << 6
	c := uint16(m[3]) & 0x3 << 14
	return a | b | c
}

// ForeignKey returns the foreign key of the memo.
func (m Memo) ForeignKey() (fk []byte) {
	fk = make([]byte, 29)
	for i := 0; i < 28; i++ {
		fk[i] |= m[i+3] >> 2
		fk[i] |= m[i+4] & 0x3 << 6
	}

	// We only have 230 bits, which results in
	// our last fk byte only having 6 'valid' bits
	fk[28] = m[31] >> 2

	return fk
}
