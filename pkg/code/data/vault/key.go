package vault

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/mr-tron/base58/base58"
)

var (
	ErrKeyNotFound         = errors.New("no records could be found")
	ErrInvalidKey          = errors.New("invalid key")
	ErrInvalidPrefixLength = errors.New("invalid prefix length")
)

type State uint

const (
	StateUnknown State = iota
	StateAvailable
	StateReserved
	StateDeprecated
	StateRevoked
)

type Record struct {
	Id uint64

	PublicKey  string
	PrivateKey string

	State State

	CreatedAt time.Time
}

func CreateKey() (*Record, error) {
	return GrindKey("")
}

func GrindKey(prefix string) (*Record, error) {
	if len(prefix) > 5 {
		return nil, ErrInvalidPrefixLength
	}

	var priv ed25519.PrivateKey
	var pub ed25519.PublicKey
	var err error

	for {
		pub, priv, err = ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}

		if base58.Encode(pub)[:len(prefix)] == prefix {
			break
		}
	}

	return &Record{
		PublicKey:  base58.Encode(pub[:]),
		PrivateKey: base58.Encode(priv[:]),
		State:      StateAvailable,
	}, nil
}

func (r *Record) IsAvailable() bool {
	return r.State == StateReserved
}

func (r *Record) IsReserved() bool {
	return r.State == StateReserved
}

func (r *Record) IsDeprecated() bool {
	return r.State == StateDeprecated
}

func (r *Record) IsRevoked() bool {
	return r.State == StateRevoked
}

func (r *Record) GetPublicKey() (ed25519.PublicKey, error) {
	return base58.Decode(r.PublicKey)
}

func (r *Record) GetPrivateKey() (ed25519.PrivateKey, error) {
	return base58.Decode(r.PrivateKey)
}

func (r *Record) Clone() Record {
	return Record{
		Id:         r.Id,
		PublicKey:  r.PublicKey,
		PrivateKey: r.PrivateKey,
		State:      r.State,
		CreatedAt:  r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.PublicKey = r.PublicKey
	dst.PrivateKey = r.PrivateKey
	dst.State = r.State
	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.PublicKey) == 0 {
		return errors.New("public key is required")
	}

	if len(r.PrivateKey) == 0 {
		return errors.New("private key is required")
	}

	return nil
}
