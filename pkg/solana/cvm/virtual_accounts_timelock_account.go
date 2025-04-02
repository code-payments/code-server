package cvm

import (
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const VirtualTimelockAccountSize = (32 + // owner
	32 + // nonce
	1 + // token_bump
	1 + // unlock_bump
	1 + // withdraw_bump
	8 + // balance
	1) // bump

type VirtualTimelockAccount struct {
	Owner ed25519.PublicKey
	Nonce Hash

	TokenBump    uint8
	UnlockBump   uint8
	WithdrawBump uint8

	Balance uint64
	Bump    uint8
}

func (obj *VirtualTimelockAccount) Marshal() []byte {
	data := make([]byte, VirtualTimelockAccountSize)

	var offset int

	putKey(data, obj.Owner, &offset)
	putHash(data, obj.Nonce, &offset)
	putUint8(data, obj.TokenBump, &offset)
	putUint8(data, obj.UnlockBump, &offset)
	putUint8(data, obj.WithdrawBump, &offset)
	putUint64(data, obj.Balance, &offset)
	putUint8(data, obj.Bump, &offset)

	return data
}

func (obj *VirtualTimelockAccount) UnmarshalDirectly(data []byte) error {
	if len(data) < VirtualTimelockAccountSize {
		return ErrInvalidVirtualAccountData
	}

	var offset int

	getKey(data, &obj.Owner, &offset)
	getHash(data, &obj.Nonce, &offset)
	getUint8(data, &obj.TokenBump, &offset)
	getUint8(data, &obj.UnlockBump, &offset)
	getUint8(data, &obj.WithdrawBump, &offset)
	getUint64(data, &obj.Balance, &offset)
	getUint8(data, &obj.Bump, &offset)

	return nil
}

func (obj *VirtualTimelockAccount) UnmarshalFromMemory(data []byte) error {
	if len(data) == 0 {
		return ErrInvalidVirtualAccountData
	}

	if data[0] != uint8(VirtualAccountTypeTimelock) {
		return ErrInvalidVirtualAccountType
	}

	return obj.UnmarshalDirectly(data[1:])
}

func (obj *VirtualTimelockAccount) String() string {
	return fmt.Sprintf(
		"VirtualTimelockAccount{owner=%s,nonce=%s,token_bump=%d,unlock_bump=%d,withdraw_bump=%d,balance=%d,bump=%d}",
		base58.Encode(obj.Owner),
		obj.Nonce.String(),
		obj.TokenBump,
		obj.UnlockBump,
		obj.WithdrawBump,
		obj.Balance,
		obj.Bump,
	)
}
