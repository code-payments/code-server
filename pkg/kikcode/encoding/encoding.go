package encoding

// todo: There are some linking issues if we don't have all the C/C++ files in
//       the current directory, but this makes the package extremely ugly.

// #cgo CXXFLAGS: -I${SRCDIR}
// #cgo CXXFLAGS: -std=c++11
// #include "kikcode_wrapper.h"
import "C"
import (
	"unsafe"

	"github.com/pkg/errors"
)

const (
	payloadSize        = 20
	encodedPayloadSize = 35
)

func Encode(payload []byte) ([]byte, error) {
	if len(payload) != payloadSize {
		return nil, errors.Errorf("payload value must be a byte array of size %d", payloadSize)
	}

	charArray := C.CString(string(payload))
	defer C.free(unsafe.Pointer(charArray))

	encoded := C.kikcode_encode(
		charArray,
		C.int(len(payload)),
	)
	return []byte(C.GoStringN(encoded, encodedPayloadSize)), nil
}

func Decode(encoded []byte) ([]byte, error) {
	if len(encoded) != encodedPayloadSize {
		return nil, errors.Errorf("encoded value must be a byte array of size %d", encodedPayloadSize)
	}

	charArray := C.CString(string(encoded))
	defer C.free(unsafe.Pointer(charArray))

	decoded := C.kikcode_decode(
		charArray,
		C.int(len(encoded)),
	)
	return []byte(C.GoStringN(decoded, payloadSize)), nil
}
