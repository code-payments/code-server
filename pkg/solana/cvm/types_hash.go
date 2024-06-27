package cvm

import (
	"encoding/hex"
)

const HashSize = 32

type Hash [HashSize]byte

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func getHash(src []byte, dst *Hash, offset *int) {
	copy(dst[:], src[*offset:])
	*offset += HashSize
}
