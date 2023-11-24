package shortvec

import (
	"fmt"
	"io"
	"math"
)

// EncodeLen encodes the specified len into the writer.
//
// If len > math.MaxUint16, an error is returned.
func EncodeLen(w io.Writer, len int) (n int, err error) {
	if len > math.MaxUint16 {
		return 0, fmt.Errorf("len exceeds %d", math.MaxUint16)
	}

	written := 0
	valBuf := make([]byte, 1)

	for {
		valBuf[0] = byte(len & 0x7f)
		len >>= 7
		if len == 0 {
			n, err := w.Write(valBuf)
			written += n

			return written, err
		}

		valBuf[0] |= 0x80
		n, err := w.Write(valBuf)
		written += n
		if err != nil {
			return written, err
		}
	}
}

// DecodeLen decodes a shortvec encoded len from the reader.
func DecodeLen(r io.Reader) (val int, err error) {
	var offset int
	valBuf := make([]byte, 1)

	for {
		if _, err := r.Read(valBuf); err != nil {
			return 0, err
		}

		val |= int(valBuf[0]&0x7f) << (offset * 7)
		offset++

		if valBuf[0]&0x80 == 0 {
			break
		}
	}

	if offset > 3 {
		return 0, fmt.Errorf("invalid size: %d (max 3)", offset)
	}

	return val, nil
}
