package address_lookup_table

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/mr-tron/base58"

	"github.com/code-payments/code-server/pkg/solana/binary"
)

var (
	ErrInvalidAccountSize = errors.New("invalid address lookup table account size")
)

const optionSize = 1

const metadataSize = 56
const maxAddresses = 256

type AddressLookupTableAccount struct {
	TypeIndex                  uint32
	DeactivationSlot           uint64
	LastExtendedSlot           uint64
	LastExtendedSlotStartIndex uint8
	Authority                  ed25519.PublicKey
	Addresses                  []ed25519.PublicKey
}

func (obj *AddressLookupTableAccount) Unmarshal(data []byte) error {
	var offset int

	if len(data) < metadataSize {
		return ErrInvalidAccountSize
	}

	binary.GetUint32(data[offset:], &obj.TypeIndex, &offset)
	binary.GetUint64(data[offset:], &obj.DeactivationSlot, &offset)
	binary.GetUint64(data[offset:], &obj.LastExtendedSlot, &offset)
	binary.GetUint8(data[offset:], &obj.LastExtendedSlotStartIndex, &offset)
	binary.GetOptionalKey32(data[offset:], &obj.Authority, &offset, optionSize)

	fmt.Println(base58.Encode(obj.Authority))

	offset = 56

	addressBufferSize := len(data) - offset
	addressCount := addressBufferSize / 32
	if addressBufferSize%ed25519.PublicKeySize != 0 {
		return ErrInvalidAccountSize
	} else if addressCount > maxAddresses {
		return ErrInvalidAccountSize
	}

	obj.Addresses = make([]ed25519.PublicKey, addressCount)
	for i := 0; i < addressCount; i++ {
		binary.GetKey32(data[offset:], &obj.Addresses[i], &offset)
	}

	return nil
}
