package cvm

import (
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const VirtualRelayAccountSize = (32 + // target
	32) // destination

type VirtualRelayAccount struct {
	Target      ed25519.PublicKey
	Destination ed25519.PublicKey
}

func (obj *VirtualRelayAccount) Marshal() []byte {
	data := make([]byte, VirtualRelayAccountSize)

	var offset int

	putKey(data, obj.Target, &offset)
	putKey(data, obj.Destination, &offset)

	return data
}

func (obj *VirtualRelayAccount) UnmarshalDirectly(data []byte) error {
	if len(data) < VirtualRelayAccountSize {
		return ErrInvalidVirtualAccountData
	}

	var offset int

	getKey(data, &obj.Target, &offset)
	getKey(data, &obj.Destination, &offset)

	return nil
}

func (obj *VirtualRelayAccount) UnmarshalFromMemory(data []byte) error {
	if len(data) == 0 {
		return ErrInvalidVirtualAccountData
	}

	if data[0] != uint8(VirtualAccountTypeRelay) {
		return ErrInvalidVirtualAccountType
	}

	return obj.UnmarshalDirectly(data[1:])
}

func (obj *VirtualRelayAccount) String() string {
	return fmt.Sprintf(
		"VirtualRelayAccount{target=%s,destination=%s}",
		base58.Encode(obj.Target),
		base58.Encode(obj.Destination),
	)
}
