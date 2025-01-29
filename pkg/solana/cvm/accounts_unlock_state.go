package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	UnlockStateAccountSize = (8 + //discriminator
		32 + // vm
		32 + // owner
		32 + // address
		1 + // bump
		1 + // state
		6) // padding
)

var UnlockStateAccountDiscriminator = []byte{byte(AccountTypeUnlockState), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type UnlockStateAccount struct {
	Vm       ed25519.PublicKey
	Owner    ed25519.PublicKey
	Address  ed25519.PublicKey
	UnlockAt int64
	Bump     uint8
	State    TimelockState
}

func (obj *UnlockStateAccount) Unmarshal(data []byte) error {
	if len(data) < UnlockStateAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, UnlockStateAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getKey(data, &obj.Owner, &offset)
	getKey(data, &obj.Address, &offset)
	getInt64(data, &obj.UnlockAt, &offset)
	getUint8(data, &obj.Bump, &offset)
	getTimelockState(data, &obj.State, &offset)
	offset += 6 // padding

	return nil
}

func (obj *UnlockStateAccount) IsUnlocked() bool {
	return obj.State == TimelockStateUnlocked
}

func (obj *UnlockStateAccount) String() string {
	return fmt.Sprintf(
		"UnlockStateAccount{vm=%s,owner=%s,address=%s,unlock_at=%d,bump=%d,state=%s",
		base58.Encode(obj.Vm),
		base58.Encode(obj.Owner),
		base58.Encode(obj.Address),
		obj.UnlockAt,
		obj.Bump,
		obj.State.String(),
	)
}
