package common

import (
	"crypto/ed25519"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
)

type Key struct {
	bytesValue  []byte
	stringValue string
}

func NewKeyFromBytes(value []byte) (*Key, error) {
	k := &Key{
		bytesValue:  value,
		stringValue: base58.Encode(value),
	}

	if err := k.Validate(); err != nil {
		return nil, err
	}
	return k, nil
}

func NewKeyFromString(value string) (*Key, error) {
	bytesValue, err := base58.Decode(value)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding string as base58")
	}

	k := &Key{
		bytesValue:  bytesValue,
		stringValue: value,
	}

	if err := k.Validate(); err != nil {
		return nil, err
	}
	return k, nil
}

func NewRandomKey() (*Key, error) {
	_, privateKeyBytes, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, errors.Wrap(err, "error generating private key")
	}

	return NewKeyFromBytes(privateKeyBytes)
}

func (k *Key) ToBytes() []byte {
	return k.bytesValue
}

func (k *Key) ToBase58() string {
	return k.stringValue
}

func (k *Key) IsPublic() bool {
	return len(k.bytesValue) != ed25519.PrivateKeySize
}

func (k *Key) Validate() error {
	if k == nil {
		return errors.New("key is nil")
	}

	if len(k.bytesValue) != ed25519.PublicKeySize && len(k.bytesValue) != ed25519.PrivateKeySize {
		return errors.New("key must be an ed25519 public or private key")
	}

	if base58.Encode(k.bytesValue) != k.stringValue {
		return errors.New("bytes and string representation don't match")
	}

	return nil
}
