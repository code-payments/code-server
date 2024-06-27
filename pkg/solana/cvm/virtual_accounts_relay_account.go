package cvm

import (
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const VirtualRelayAccountSize = (32 + // address
	32 + // commitment
	32 + // recent_root
	32) // destination

type VirtualRelayAccount struct {
	Address     ed25519.PublicKey
	Commitment  Hash
	RecentRoot  Hash
	Destination ed25519.PublicKey
}

func (obj *VirtualRelayAccount) UnmarshalDirectly(data []byte) error {
	if len(data) < VirtualRelayAccountSize {
		return ErrInvalidVirtualAccountData
	}

	var offset int

	getKey(data, &obj.Address, &offset)
	getHash(data, &obj.Commitment, &offset)
	getHash(data, &obj.RecentRoot, &offset)
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
		"VirtualRelayAccount{address=%s,commitment=%s,recent_root=%s,destination=%s}",
		base58.Encode(obj.Address),
		obj.Commitment.String(),
		obj.RecentRoot.String(),
		base58.Encode(obj.Destination),
	)
}
