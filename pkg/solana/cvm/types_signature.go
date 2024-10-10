package cvm

import (
	"github.com/mr-tron/base58"
)

const SignatureSize = 64

type Signature [SignatureSize]byte

func (s Signature) String() string {
	return base58.Encode(s[:])
}

func putSignature(dst []byte, v Signature, offset *int) {
	copy(dst[*offset:], v[:])
	*offset += SignatureSize
}
func getSignature(src []byte, dst *Signature, offset *int) {
	copy(dst[:], src[*offset:])
	*offset += SignatureSize
}
