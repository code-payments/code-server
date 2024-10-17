package cvm

import (
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	TokenPoolSize = (32 + // vault
		1) // vault_bump
)

type TokenPool struct {
	Vault     ed25519.PublicKey
	VaultBump uint8
}

func (obj *TokenPool) Unmarshal(data []byte) error {
	if len(data) < TokenPoolSize {
		return ErrInvalidAccountData
	}

	var offset int

	getKey(data, &obj.Vault, &offset)
	getUint8(data, &obj.VaultBump, &offset)

	return nil
}

func (obj *TokenPool) String() string {
	return fmt.Sprintf(
		"TokenPool{vault=%s,vault_bump=%d}",
		base58.Encode(obj.Vault),
		obj.VaultBump,
	)
}

func getTokenPool(src []byte, dst *TokenPool, offset *int) {
	dst.Unmarshal(src[*offset:])
	*offset += TokenPoolSize
}
