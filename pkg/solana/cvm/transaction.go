package cvm

import (
	"crypto/ed25519"
)

// todo: Utilities for crafting a transaction with virtual instructions, but this may be too low-level of a package for that

func getOptionalAccountMetaAddress(account *ed25519.PublicKey) ed25519.PublicKey {
	if account != nil {
		return *account
	}
	return PROGRAM_ID
}
