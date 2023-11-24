package solana

import (
	"crypto/ed25519"
	"crypto/sha256"
	"math"

	"github.com/jdgcs/ed25519/edwards25519"
	"github.com/pkg/errors"
)

const (
	maxSeeds      = 16
	maxSeedLength = 32
)

var (
	ErrTooManySeeds          = errors.New("too many seeds")
	ErrMaxSeedLengthExceeded = errors.New("max seed length exceeded")

	ErrInvalidPublicKey = errors.New("invalid public key")
)

var (
	programHashCtor = sha256.New
)

// CreateProgramAddress mirrors the implementation of the Solana SDK's CreateProgramAddress.
//
// ProgramAddresses are public keys that _do not_ lie on the ed25519 curve to ensure that
// there is no associated private key. In the event that the program and seed parameters
// result in a valid public key, ErrInvalidPublicKey is returned.
//
// Reference: https://github.com/solana-labs/solana/blob/5548e599fe4920b71766e0ad1d121755ce9c63d5/sdk/program/src/pubkey.rs#L158
func CreateProgramAddress(program ed25519.PublicKey, seeds ...[]byte) (ed25519.PublicKey, error) {
	if len(seeds) > maxSeeds {
		return nil, ErrTooManySeeds
	}

	h := programHashCtor()
	for _, s := range seeds {
		if len(s) > maxSeedLength {
			return nil, ErrMaxSeedLengthExceeded
		}

		if _, err := h.Write(s); err != nil {
			return nil, errors.Wrap(err, "failed to hash seed")
		}
	}

	for _, v := range [][]byte{program, []byte("ProgramDerivedAddress")} {
		if _, err := h.Write(v); err != nil {
			return nil, errors.Wrap(err, "failed to hash seed")
		}
	}

	hash := h.Sum(nil)
	var pub [32]byte
	copy(pub[:], hash)

	// Following the Solana SDK, we want to _reject_ the generated public key
	// if it's a valid compressed EdwardsPoint.
	//
	// Unfortunately, we don't have a _direct_ equivalent in Go, but reverse
	// engineering the ed25519.Verify() method exposes how the library determines
	// a valid public key.
	//
	// Unfortunately, the edwards25519.ExtendedGroupElement (the EdwardsPoint) is
	// internal to the golang.org/x/crypto library, so we rely (for now) on a
	// deprecated open source alternative.
	//
	// Reference: https://github.com/solana-labs/solana/blob/5548e599fe4920b71766e0ad1d121755ce9c63d5/sdk/program/src/pubkey.rs#L182-L187
	var A edwards25519.ExtendedGroupElement
	if A.FromBytes(&pub) {
		return nil, ErrInvalidPublicKey
	}

	return pub[:], nil
}

// FindProgramAddressAndBump mirrors the implementation of the Solana SDK's
// FindProgramAddress. It returns the address and bump seed.
//
// Reference: https://github.com/solana-labs/solana/blob/5548e599fe4920b71766e0ad1d121755ce9c63d5/sdk/program/src/pubkey.rs#L234
func FindProgramAddressAndBump(program ed25519.PublicKey, seeds ...[]byte) (ed25519.PublicKey, uint8, error) {
	bumpSeed := []byte{math.MaxUint8}
	for i := 0; i < math.MaxUint8; i++ {
		pub, err := CreateProgramAddress(program, append(seeds, bumpSeed)...)
		if err == nil {
			return pub, bumpSeed[0], nil
		}
		if err != ErrInvalidPublicKey {
			return nil, 0, err
		}

		bumpSeed[0]--
	}

	return nil, 0, nil
}

// FindProgramAddress mirrors the implementation of the Solana SDK's FindProgramAddress.
// It only returns the address.
//
// Reference: https://github.com/solana-labs/solana/blob/5548e599fe4920b71766e0ad1d121755ce9c63d5/sdk/program/src/pubkey.rs#L234
func FindProgramAddress(program ed25519.PublicKey, seeds ...[]byte) (ed25519.PublicKey, error) {
	pub, _, err := FindProgramAddressAndBump(program, seeds...)
	return pub, err
}
