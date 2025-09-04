package currencycreator

import (
	"crypto/ed25519"
	"encoding/binary"
	"strings"

	"github.com/mr-tron/base58"
)

func ToQuarks(amount uint64) uint64 {
	return amount * DefaultMintQuarksPerUnit
}

func getDiscriminator(src []byte, dst *[]byte, offset *int) {
	*dst = make([]byte, 8)
	copy(*dst, src[*offset:])
	*offset += 8
}

func putKey(dst []byte, v ed25519.PublicKey, offset *int) {
	copy(dst[*offset:], v)
	*offset += ed25519.PublicKeySize
}
func getKey(src []byte, dst *ed25519.PublicKey, offset *int) {
	*dst = make([]byte, ed25519.PublicKeySize)
	copy(*dst, src[*offset:])
	*offset += ed25519.PublicKeySize
}

func putFixedString(dst []byte, v string, length int, offset *int) {
	copy(dst[*offset:], toFixedString(v, length))
	*offset += length
}
func getFixedString(data []byte, dst *string, length int, offset *int) {
	*dst = string(data[*offset : *offset+length])
	*dst = removeFixedStringPadding(*dst)
	*offset += length
}

func putUint8(dst []byte, v uint8, offset *int) {
	dst[*offset] = v
	*offset += 1
}
func getUint8(src []byte, dst *uint8, offset *int) {
	*dst = src[*offset]
	*offset += 1
}

func putUint16(dst []byte, v uint16, offset *int) {
	binary.LittleEndian.PutUint16(dst[*offset:], v)
	*offset += 2
}
func getUint16(src []byte, dst *uint16, offset *int) {
	*dst = binary.LittleEndian.Uint16(src[*offset:])
	*offset += 2
}

func putUint64(dst []byte, v uint64, offset *int) {
	binary.LittleEndian.PutUint64(dst[*offset:], v)
	*offset += 8
}

func toFixedString(value string, length int) string {
	fixed := make([]byte, length)
	copy(fixed, []byte(value))
	return string(fixed)
}
func removeFixedStringPadding(value string) string {
	return strings.TrimRight(value, string([]byte{0}))
}

func mustBase58Decode(value string) []byte {
	decoded, err := base58.Decode(value)
	if err != nil {
		panic(err)
	}
	return decoded
}
