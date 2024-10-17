package cvm

import (
	"crypto/ed25519"
)

func getOptionalAccountMetaAddress(account *ed25519.PublicKey) ed25519.PublicKey {
	if account != nil {
		return *account
	}
	return PROGRAM_ID
}
