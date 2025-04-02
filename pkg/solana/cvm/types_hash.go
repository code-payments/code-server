package cvm

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const HashSize = 32

type Hash [HashSize]byte

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func putHash(dst []byte, v Hash, offset *int) {
	copy(dst[*offset:], v[:])
	*offset += HashSize
}
func getHash(src []byte, dst *Hash, offset *int) {
	copy(dst[:], src[*offset:])
	*offset += HashSize
}

type HashArray []Hash

func getStaticHashArray(src []byte, dst *HashArray, length int, offset *int) {
	*dst = make([]Hash, length)
	for i := 0; i < int(length); i++ {
		getHash(src, &(*dst)[i], offset)
	}
}

func putHashArray(dst []byte, v HashArray, offset *int) {
	binary.LittleEndian.PutUint32(dst[*offset:], uint32(len(v)))
	*offset += 4

	for _, hash := range v {
		copy(dst[*offset:], hash[:])
		*offset += HashSize
	}
}

func (h HashArray) String() string {
	stringValues := make([]string, len(h))
	for i := 0; i < len(h); i++ {
		stringValues[i] = h[i].String()
	}
	return fmt.Sprintf("[%s]", strings.Join(stringValues, ","))
}
