package splitter_token

import "fmt"

const (
	// HashSize is the size, in bytes, of SHA256 hashes as used in this package.
	HashSize = 32
)

type Hash []byte

func (h Hash) ToString() string {
	return fmt.Sprintf("%x", h)
}

func putHash(dst []byte, src []byte, offset *int) {
	copy(dst[*offset:], src)
	*offset += HashSize
}

func getHash(src []byte, dst *Hash, offset *int) {
	*dst = make([]byte, HashSize)
	copy(*dst, src[*offset:])
	*offset += HashSize
}
