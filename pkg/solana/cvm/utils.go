package cvm

import (
	"crypto/ed25519"
	"encoding/binary"
	"strings"

	"github.com/mr-tron/base58"
)

func putDiscriminator(dst []byte, v []byte, offset *int) {
	copy(dst[*offset:], v)
	*offset += 8
}
func getDiscriminator(src []byte, dst *[]byte, offset *int) {
	*dst = make([]byte, 8)
	copy(*dst, src[*offset:])
	*offset += 8
}

func getKey(src []byte, dst *ed25519.PublicKey, offset *int) {
	*dst = make([]byte, ed25519.PublicKeySize)
	copy(*dst, src[*offset:])
	*offset += ed25519.PublicKeySize
}

func getBool(src []byte, dst *bool, offset *int) {
	if src[*offset] == 1 {
		*dst = true
	} else {
		*dst = false
	}
	*offset += 1
}

func putString(dst []byte, src string, offset *int) {
	putUint32(dst, uint32(len(src)), offset)
	copy(dst[*offset:], src)
	*offset += len(src)
}

func getFixedString(data []byte, dst *string, length int, offset *int) {
	*dst = string(data[*offset : *offset+length])
	*dst = removeFixedStringPadding(*dst)
	*offset += length
}

func getData(src []byte, dst []byte, length int, offset *int) {
	copy(dst[:length], src[*offset:*offset+length])
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

func getUint16(src []byte, dst *uint16, offset *int) {
	*dst = binary.LittleEndian.Uint16(src[*offset:])
	*offset += 2
}

func putUint32(dst []byte, v uint32, offset *int) {
	binary.LittleEndian.PutUint32(dst[*offset:], v)
	*offset += 4
}

func getUint64(src []byte, dst *uint64, offset *int) {
	*dst = binary.LittleEndian.Uint64(src[*offset:])
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
