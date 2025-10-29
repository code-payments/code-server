package binary

import (
	"crypto/ed25519"
	"encoding/binary"
)

func PutKey32(dst []byte, src []byte, offset *int) {
	copy(dst, src)
	*offset += ed25519.PublicKeySize
}

func PutOptionalKey32(dst []byte, src []byte, offset *int, optionSize int) {
	if len(src) > 0 {
		dst[0] = 1
		copy(dst[optionSize:], src)
	}

	*offset += optionSize + ed25519.PublicKeySize
}

func PutUint64(dst []byte, v uint64, offset *int) {
	binary.LittleEndian.PutUint64(dst, v)
	*offset += 8
}

func PutUint32(dst []byte, v uint32, offset *int) {
	binary.LittleEndian.PutUint32(dst, v)
	*offset += 4
}

func PutUint8(dst []byte, v uint8, offset *int) {
	dst[0] = v
	*offset += 1
}

func PutOptionalUint64(dst []byte, v *uint64, offset *int, optionSize int) {
	if v != nil {
		dst[0] = 1
		binary.LittleEndian.PutUint64(dst[optionSize:], *v)
	}
	*offset += optionSize + 8
}

func GetKey32(src []byte, dst *ed25519.PublicKey, offset *int) {
	*dst = make([]byte, ed25519.PublicKeySize)
	copy(*dst, src)
	*offset += ed25519.PublicKeySize
}

func GetOptionalKey32(src []byte, dst *ed25519.PublicKey, offset *int, optionSize int) {
	if src[0] == 1 {
		*dst = make([]byte, ed25519.PublicKeySize)
		copy(*dst, src[optionSize:])
	}
	*offset += optionSize + ed25519.PublicKeySize
}

func GetUint64(src []byte, dst *uint64, offset *int) {
	*dst = binary.LittleEndian.Uint64(src)
	*offset += 8
}

func GetUint32(src []byte, dst *uint32, offset *int) {
	*dst = binary.LittleEndian.Uint32(src)
	*offset += 4
}

func GetUint8(src []byte, dst *uint8, offset *int) {
	*dst = src[0]
	*offset += 1
}

func GetOptionalUint64(src []byte, dst **uint64, offset *int, optionSize int) {
	if src[0] == 1 {
		val := binary.LittleEndian.Uint64(src[optionSize:])
		*dst = &val
	}
	*offset += optionSize + 8
}
