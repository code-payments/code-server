package solana

import (
	"bytes"
	"crypto/ed25519"
	"io"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/solana/shortvec"
)

const (
	messageVersionSerializationOffset = 127
)

func (s TransactionSignature) ToBase58() string {
	return base58.Encode(s.Signature[:])
}

func (t Transaction) Marshal() []byte {
	b := bytes.NewBuffer(nil)

	// Signatures
	_, _ = shortvec.EncodeLen(b, len(t.Signatures))
	for _, s := range t.Signatures {
		_, _ = b.Write(s[:])
	}

	// Message
	_, _ = b.Write(t.Message.Marshal())

	return b.Bytes()
}

func (t *Transaction) Unmarshal(b []byte) error {
	buf := bytes.NewBuffer(b)

	sigLen, err := shortvec.DecodeLen(buf)
	if err != nil {
		return errors.Wrap(err, "failed to read signature length")
	}

	t.Signatures = make([]Signature, sigLen)
	for i := 0; i < sigLen; i++ {
		if _, err = io.ReadFull(buf, t.Signatures[i][:]); err != nil {
			return errors.Wrapf(err, "failed to read signature at %d", i)
		}
	}

	return (&t.Message).Unmarshal(buf.Bytes())
}

func (m Message) Marshal() []byte {
	buf := bytes.NewBuffer(nil)
	switch m.Version {
	case MessageVersionLegacy:
		m.marshalLegacy(buf)
	case MessageVersion0:
		m.marshalV0(buf)
	default:
		panic("unsupported message version")
	}
	return buf.Bytes()
}

func (m *Message) marshalLegacy(b *bytes.Buffer) {
	// Header
	_ = b.WriteByte(m.Header.NumSignatures)
	_ = b.WriteByte(m.Header.NumReadonlySigned)
	_ = b.WriteByte(m.Header.NumReadOnly)

	// Accounts
	_, _ = shortvec.EncodeLen(b, len(m.Accounts))
	for _, a := range m.Accounts {
		_, _ = b.Write(a)
	}

	// Recent Blockhash
	_, _ = b.Write(m.RecentBlockhash[:])

	// Instructions
	_, _ = shortvec.EncodeLen(b, len(m.Instructions))
	for _, i := range m.Instructions {
		_ = b.WriteByte(i.ProgramIndex)

		// Accounts
		_, _ = shortvec.EncodeLen(b, len(i.Accounts))
		_, _ = b.Write(i.Accounts)

		// Data
		_, _ = shortvec.EncodeLen(b, len(i.Data))
		_, _ = b.Write(i.Data)
	}
}

func (m *Message) marshalV0(b *bytes.Buffer) {
	// Version Number
	_ = b.WriteByte(byte(m.Version + messageVersionSerializationOffset))

	// Message Content
	//
	// Note: The "middle" section remains the same as legacy format
	m.marshalLegacy(b)

	// Address Table Lookups
	_, _ = shortvec.EncodeLen(b, len(m.AddressTableLookups))
	for _, addressTableLookup := range m.AddressTableLookups {
		_, _ = b.Write(addressTableLookup.PublicKey)

		_, _ = shortvec.EncodeLen(b, len(addressTableLookup.WritableIndexes))
		_, _ = b.Write(addressTableLookup.WritableIndexes)

		_, _ = shortvec.EncodeLen(b, len(addressTableLookup.ReadonlyIndexes))
		_, _ = b.Write(addressTableLookup.ReadonlyIndexes)
	}
}

func (m *Message) Unmarshal(b []byte) (err error) {
	if len(b) == 0 {
		return errors.New("invalid byte buffer")
	}

	if b[0] < messageVersionSerializationOffset {
		m.Version = MessageVersionLegacy
	} else if b[0] == byte(MessageVersion0+messageVersionSerializationOffset) {
		m.Version = MessageVersion0
	} else {
		return errors.New("unsupported message version")
	}

	buf := bytes.NewBuffer(b)

	switch m.Version {
	case MessageVersionLegacy:
		return m.unmarshalLegacy(buf)
	case MessageVersion0:
		return m.unmarshalV0(buf)
	default:
		return errors.New("unsupported message version")
	}
}

func (m *Message) unmarshalLegacy(buf *bytes.Buffer) (err error) {
	// Header
	if m.Header.NumSignatures, err = buf.ReadByte(); err != nil {
		return errors.Wrap(err, "failed to read num signatures")
	}
	if m.Header.NumReadonlySigned, err = buf.ReadByte(); err != nil {
		return errors.Wrap(err, "failed to read num readonly signatures")
	}
	if m.Header.NumReadOnly, err = buf.ReadByte(); err != nil {
		return errors.Wrap(err, "failed to read num readonly")
	}

	// Accounts
	accountLen, err := shortvec.DecodeLen(buf)
	if err != nil {
		return errors.Wrap(err, "failed to read account len")
	}
	m.Accounts = make([]ed25519.PublicKey, accountLen)
	for i := 0; i < accountLen; i++ {
		m.Accounts[i] = make([]byte, ed25519.PublicKeySize)
		if _, err = io.ReadFull(buf, m.Accounts[i]); err != nil {
			return errors.Wrapf(err, "failed to read account at index %d", i)
		}
	}

	// Recent blockhash
	if _, err = io.ReadFull(buf, m.RecentBlockhash[:]); err != nil {
		return errors.Wrap(err, "failed to read recent block hash")
	}

	// Instructions
	instructionLen, err := shortvec.DecodeLen(buf)
	if err != nil {
		return errors.Wrap(err, "failed to read instruction len")
	}
	m.Instructions = make([]CompiledInstruction, instructionLen)
	for i := 0; i < instructionLen; i++ {
		var c CompiledInstruction

		// Program Index
		if c.ProgramIndex, err = buf.ReadByte(); err != nil {
			return errors.Wrapf(err, "failed to read instruction[%d] program index", i)
		}
		if int(c.ProgramIndex) >= len(m.Accounts) {
			return errors.Errorf("program index out of range: %d:%d", i, c.ProgramIndex)
		}

		// Account Indexes
		accountLen, err = shortvec.DecodeLen(buf)
		if err != nil {
			return errors.Wrapf(err, "failed to read instruction[%d] account len", i)
		}
		c.Accounts = make([]byte, accountLen)
		if _, err = io.ReadFull(buf, c.Accounts); err != nil {
			return errors.Wrapf(err, "failed to read instruction[%d] accounts", i)
		}

		for _, index := range c.Accounts {
			if int(index) >= len(m.Accounts) && m.Version == MessageVersionLegacy {
				return errors.Errorf("account index out of range: %d:%d", i, index)
			}
		}

		// Data
		dataLen, err := shortvec.DecodeLen(buf)
		if err != nil {
			return errors.Wrapf(err, "failed to read instruction[%d] data len", i)
		}
		c.Data = make([]byte, dataLen)
		if _, err = io.ReadFull(buf, c.Data); err != nil {
			return errors.Wrapf(err, "failed to read instruction[%d] data", i)
		}

		m.Instructions[i] = c
	}

	return nil
}

func (m *Message) unmarshalV0(buf *bytes.Buffer) (err error) {
	// Message Version
	version, err := buf.ReadByte()
	if err != nil {
		return errors.Wrap(err, "failed to read version byte")
	}
	if version != byte(MessageVersion0+messageVersionSerializationOffset) {
		return errors.New("message version is not v0")
	}

	// Message Content
	//
	// Note: The "middle" section remains the same as legacy format
	err = m.unmarshalLegacy(buf)
	if err != nil {
		return err
	}

	// Address Table Lookups
	addressTableLookupLen, err := shortvec.DecodeLen(buf)
	if err != nil {
		return errors.Wrap(err, "failed to read address table lookup len")
	}

	m.AddressTableLookups = make([]MessageAddressTableLookup, addressTableLookupLen)
	for i := range addressTableLookupLen {
		// Public Key
		m.AddressTableLookups[i].PublicKey = make([]byte, ed25519.PublicKeySize)
		if _, err = io.ReadFull(buf, m.AddressTableLookups[i].PublicKey); err != nil {
			return errors.Wrapf(err, "failed to read address table lookup[%d] public key", i)
		}

		// Writeable indexes
		writableIndexesLen, err := shortvec.DecodeLen(buf)
		if err != nil {
			return errors.Wrapf(err, "failed to read address table lookup[%d] writable indexes len", i)
		}
		m.AddressTableLookups[i].WritableIndexes = make([]byte, writableIndexesLen)
		if _, err = io.ReadFull(buf, m.AddressTableLookups[i].WritableIndexes); err != nil {
			return errors.Wrapf(err, "failed to read address table lookup[%d] writeable indexes", i)
		}

		// Readonly indexes
		readonlyIndexesLen, err := shortvec.DecodeLen(buf)
		if err != nil {
			return errors.Wrapf(err, "failed to read address table lookup[%d] readonly indexes len", i)
		}
		m.AddressTableLookups[i].ReadonlyIndexes = make([]byte, readonlyIndexesLen)
		if _, err = io.ReadFull(buf, m.AddressTableLookups[i].ReadonlyIndexes); err != nil {
			return errors.Wrapf(err, "failed to read address table lookup[%d] readonly indexes", i)
		}
	}

	return nil
}
