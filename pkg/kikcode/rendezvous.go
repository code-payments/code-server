package kikcode

import (
	"crypto/ed25519"
	"crypto/sha256"
)

func DeriveRendezvousPrivateKey(payload *Payload) ed25519.PrivateKey {
	hashed := sha256.Sum256(payload.ToBytes())
	return ed25519.NewKeyFromSeed(hashed[:])
}
