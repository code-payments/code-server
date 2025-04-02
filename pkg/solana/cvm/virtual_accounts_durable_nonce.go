package cvm

import (
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const VirtualDurableNonceSize = (32 + // address
	32) // hash

type VirtualDurableNonce struct {
	Address ed25519.PublicKey
	Value   Hash
}

func (obj *VirtualDurableNonce) Marshal() []byte {
	data := make([]byte, VirtualDurableNonceSize)

	var offset int

	putKey(data, obj.Address, &offset)
	putHash(data, obj.Value, &offset)

	return data
}

func (obj *VirtualDurableNonce) UnmarshalDirectly(data []byte) error {
	if len(data) < VirtualDurableNonceSize {
		return ErrInvalidVirtualAccountData
	}

	var offset int

	getKey(data, &obj.Address, &offset)
	getHash(data, &obj.Value, &offset)

	return nil
}

func (obj *VirtualDurableNonce) UnmarshalFromMemory(data []byte) error {
	if len(data) == 0 {
		return ErrInvalidVirtualAccountData
	}

	if data[0] != uint8(VirtualAccountTypeDurableNonce) {
		return ErrInvalidVirtualAccountType
	}

	return obj.UnmarshalDirectly(data[1:])
}

func (obj *VirtualDurableNonce) String() string {
	return fmt.Sprintf(
		"VirtualDurableNonce{address=%s,value=%s}",
		base58.Encode(obj.Address),
		obj.Value.String(),
	)
}
