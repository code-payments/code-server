package swap_validator

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/mr-tron/base58"
)

// This file should be replaced with something more robust. Modified version of what the original agora and code pkgs do.

func putDiscriminator(dst []byte, src []byte, offset *int) {
	copy(dst[*offset:], src)
	*offset += 8
}
func getDiscriminator(src []byte, dst *[]byte, offset *int) {
	*dst = make([]byte, 8)
	copy(*dst, src[*offset:])
	*offset += 8
}

func putKey(dst []byte, src []byte, offset *int) {
	copy(dst[*offset:], src)
	*offset += ed25519.PublicKeySize
}
func getKey(src []byte, dst *ed25519.PublicKey, offset *int) {
	*dst = make([]byte, ed25519.PublicKeySize)
	copy(*dst, src[*offset:])
	*offset += ed25519.PublicKeySize
}

func putUint8(dst []byte, v uint8, offset *int) {
	dst[*offset] = v
	*offset += 1
}
func getUint8(src []byte, dst *uint8, offset *int) {
	*dst = src[*offset]
	*offset += 1
}

func putUint64(dst []byte, v uint64, offset *int) {
	binary.LittleEndian.PutUint64(dst[*offset:], v)
	*offset += 8
}
func getUint64(src []byte, dst *uint64, offset *int) {
	*dst = binary.LittleEndian.Uint64(src[*offset:])
	*offset += 8
}

func mustBase58Decode(value string) []byte {
	decoded, err := base58.Decode(value)
	if err != nil {
		panic(err)
	}
	return decoded
}
