package system

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana/binary"
	"github.com/pkg/errors"
)

type NonceVersion uint32

const (
	NonceAccountSize = 80
)

const (
	NonceVersion0 NonceVersion = iota
	NonceVersion1
)

var (
	ErrInvalidAccountSize    = errors.New("invalid nonce account size")
	ErrInvalidAccountVersion = errors.New("invalid nonce account version")
)

// https://github.com/solana-labs/solana/blob/da00b39f4f92fb16417bd2d8bd218a04a34527b8/sdk/program/src/nonce/state/current.rs#L8
type NonceAccount struct {
	Version       uint32
	State         uint32
	Authority     ed25519.PublicKey
	Blockhash     ed25519.PublicKey
	FeeCalculator FeeCalculator
}

type FeeCalculator struct {
	LamportsPerSignature uint64
}

func (obj NonceAccount) Marshal() []byte {
	res := make([]byte, NonceAccountSize)

	var offset int
	binary.PutUint32(res[offset:], obj.Version, &offset)
	binary.PutUint32(res[offset:], obj.State, &offset)
	binary.PutKey32(res[offset:], obj.Authority, &offset)
	binary.PutKey32(res[offset:], obj.Blockhash, &offset)

	binary.PutUint64(res[offset:], obj.FeeCalculator.LamportsPerSignature, &offset)

	return res
}

func (obj *NonceAccount) Unmarshal(data []byte) error {
	if len(data) != NonceAccountSize {
		return ErrInvalidAccountSize
	}

	var offset int

	binary.GetUint32(data[offset:], &obj.Version, &offset)
	binary.GetUint32(data[offset:], &obj.State, &offset)
	binary.GetKey32(data[offset:], &obj.Authority, &offset)
	binary.GetKey32(data[offset:], &obj.Blockhash, &offset)

	binary.GetUint64(data[offset:], &obj.FeeCalculator.LamportsPerSignature, &offset)

	if NonceVersion(obj.Version) != NonceVersion1 {
		return ErrInvalidAccountVersion
	}

	return nil
}
