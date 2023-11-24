package vault

import (
	"os"
)

const (
	vaultSecretKeyEnv  = "VAULT_SECRET_KEY"
	defaultVaultSecret = "DpmmM8ruhZ5C26USxxEMBfQGAJPzHen6NxNrfKkyaBTB" // for local testing
)

func GetSecret() string {
	secret := os.Getenv(vaultSecretKeyEnv)
	if len(secret) == 0 {
		secret = defaultVaultSecret
	}

	return secret
}
