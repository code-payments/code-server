package query

import (
	"encoding/binary"

	"github.com/mr-tron/base58"
)

type Cursor []byte

var (
	EmptyCursor Cursor = Cursor([]byte{})
)

func ToCursor(val uint64) Cursor {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, val)
	return b
}

func FromCursor(val []byte) uint64 {
	return binary.BigEndian.Uint64(val)
}

func (c Cursor) ToUint64() uint64 {
	return binary.BigEndian.Uint64(c)
}

func (c Cursor) ToBase58() string {
	return base58.Encode(c)
}
