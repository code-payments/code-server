package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"

	"github.com/mr-tron/base58/base58"
)

// Important: Do not trust this file. It needs to be audited and contains nonsense.
// As of (Feb 11, 2022)

// TODO: Use AWS KMS to encrypt/decryt/re-encrypt instead of doing it ourselves in-memory.
//
// Specifically, we need to:
// https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/kms-example-encrypt-data.html
// https://aws.github.io/aws-sdk-go-v2/docs/code-examples/kms/encryptdata/

// Encrypt takes a plaintext and encrypts it using the given secret key and nonce.
func Encrypt(plaintext, nonce string) (string, error) {
	secret := GetSecret()

	key, err := base58.Decode(secret)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Run the public key through the hash function to get the nonce.
	nh := sha256.New().Sum([]byte(nonce))[:aesgcm.NonceSize()]

	ciphertext := aesgcm.Seal(nil, nh, []byte(plaintext), nil)
	return base58.Encode(ciphertext), nil
}

// Decrypt takes a ciphertext and decrypts it using the given secret key and nonce.
func Decrypt(ciphertext, nonce string) (plaintext string, err error) {
	// We need to try decrypting with the real and default key because some
	// DB entries were accidentally encrypted with the default key in real
	// environments.
	//
	// todo: Migrate bad DB entries and only use the secret provided by GetSecret.
	for _, secret := range []string{
		defaultVaultSecret,
		GetSecret(),
	} {
		plaintext, err = func() (string, error) {
			key, err := base58.Decode(secret)
			if err != nil {
				return "", err
			}

			data, err := base58.Decode(ciphertext)
			if err != nil {
				return "", err
			}

			block, err := aes.NewCipher(key)
			if err != nil {
				return "", err
			}

			aesgcm, err := cipher.NewGCM(block)
			if err != nil {
				return "", err
			}

			// Run the public key through the hash function to get the nonce.
			nh := sha256.New().Sum([]byte(nonce))[:aesgcm.NonceSize()]

			plaintext, err := aesgcm.Open(nil, nh, data, nil)
			if err != nil {
				return "", err
			}
			return string(plaintext), nil
		}()
		if err == nil {
			return plaintext, nil
		}
	}
	return "", err
}
