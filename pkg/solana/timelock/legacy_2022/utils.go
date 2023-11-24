package timelock_token

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/mr-tron/base58"
)

const optionalSize = 1

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

func putUint32(dst []byte, v uint32, offset *int) {
	binary.LittleEndian.PutUint32(dst[*offset:], v)
	*offset += 4
}
func getUint32(src []byte, dst *uint32, offset *int) {
	*dst = binary.LittleEndian.Uint32(src[*offset:])
	*offset += 4
}

func putUint64(dst []byte, v uint64, offset *int) {
	binary.LittleEndian.PutUint64(dst[*offset:], v)
	*offset += 8
}
func getUint64(src []byte, dst *uint64, offset *int) {
	*dst = binary.LittleEndian.Uint64(src[*offset:])
	*offset += 8
}

func putOptionalKey(dst []byte, src []byte, offset *int) {
	if len(src) > 0 {
		dst[*offset] = 1
		copy(dst[*offset+optionalSize:], src)
		*offset += optionalSize + ed25519.PublicKeySize
	} else {
		dst[*offset] = 0
		*offset += optionalSize
	}
}
func getOptionalKey(src []byte, dst *ed25519.PublicKey, offset *int) {
	if src[*offset] == 1 {
		*dst = make([]byte, ed25519.PublicKeySize)
		copy(*dst, src[*offset+optionalSize:])
		*offset += optionalSize + ed25519.PublicKeySize
	} else {
		*offset += optionalSize
	}
}

func putOptionalUint32(dst []byte, v *uint32, offset *int) {
	if v != nil {
		dst[*offset] = 1
		binary.LittleEndian.PutUint32(dst[*offset+optionalSize:], *v)
		*offset += optionalSize + 4
	} else {
		dst[*offset] = 0
		*offset += optionalSize
	}
}
func getOptionalUint32(src []byte, dst **uint32, offset *int) {
	if src[*offset] == 1 {
		val := binary.LittleEndian.Uint32(src[*offset+optionalSize:])
		*dst = &val
		*offset += optionalSize + 4
	} else {
		*offset += optionalSize
	}
}

func putOptionalUint64(dst []byte, v *uint64, offset *int) {
	if v != nil {
		dst[*offset] = 1
		binary.LittleEndian.PutUint64(dst[*offset+optionalSize:], *v)
		*offset += optionalSize + 8
	} else {
		dst[*offset] = 0
		*offset += optionalSize
	}
}
func getOptionalUint64(src []byte, dst **uint64, offset *int) {
	if src[*offset] == 1 {
		val := binary.LittleEndian.Uint64(src[*offset+optionalSize:])
		*dst = &val
		*offset += optionalSize + 8
	} else {
		*offset += optionalSize
	}
}

func mustBase58Decode(value string) []byte {
	decoded, err := base58.Decode(value)
	if err != nil {
		panic(err)
	}
	return decoded
}
