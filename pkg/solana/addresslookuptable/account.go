package address_lookup_table

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/code-payments/code-server/pkg/solana/binary"
	"github.com/mr-tron/base58"
)

var (
	ErrInvalidAccountSize = errors.New("invalid address lookup table account size")
	ErrInvalidAccountType = errors.New("invalid account type")
)

const (
	altDescriminator = 1

	metadataSize = 56
	maxAddresses = 256

	optionSize = 1
)

type AddressLookupTableAccount struct {
	DeactivationSlot           uint64
	LastExtendedSlot           uint64
	LastExtendedSlotStartIndex uint8
	Authority                  ed25519.PublicKey
	Addresses                  []ed25519.PublicKey
}

func (obj *AddressLookupTableAccount) Unmarshal(data []byte) error {
	if len(data) < metadataSize {
		return ErrInvalidAccountSize
	}

	var offset int

	var descriminator uint32
	binary.GetUint32(data[offset:], &descriminator, &offset)
	if descriminator != altDescriminator {
		return ErrInvalidAccountType
	}

	binary.GetUint64(data[offset:], &obj.DeactivationSlot, &offset)
	binary.GetUint64(data[offset:], &obj.LastExtendedSlot, &offset)
	binary.GetUint8(data[offset:], &obj.LastExtendedSlotStartIndex, &offset)
	binary.GetOptionalKey32(data[offset:], &obj.Authority, &offset, optionSize)

	offset = metadataSize

	addressBufferSize := len(data) - offset
	addressCount := addressBufferSize / 32
	if addressBufferSize%ed25519.PublicKeySize != 0 {
		return ErrInvalidAccountSize
	} else if addressCount > maxAddresses {
		return ErrInvalidAccountSize
	}

	obj.Addresses = make([]ed25519.PublicKey, addressCount)
	for i := range addressCount {
		binary.GetKey32(data[offset:], &obj.Addresses[i], &offset)
	}

	return nil
}

func (obj *AddressLookupTableAccount) String() string {
	addressesString := "{"
	for i, address := range obj.Addresses {
		addressesString += fmt.Sprintf("%d:%s,", i, base58.Encode(address))
	}
	addressesString += "}"

	return fmt.Sprintf(
		"AddressLookupTable{deactivation_slot=%d,last_extended_slot=%d,last_extended_slot_start_index=%d,authority=%s,addresses=%s}",
		obj.DeactivationSlot,
		obj.LastExtendedSlot,
		obj.LastExtendedSlotStartIndex,
		base58.Encode(obj.Authority),
		addressesString,
	)
}
