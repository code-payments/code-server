package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	// todo: func for real size
	minCodeVmAccountSize = (8 + //discriminator
		32 + // authority
		32 + // mint
		TokenPoolSize + // omnibus
		1 + // lock_duration
		1 + // bump
		8 + // slot
		HashSize + // poh
		7) // padding
)

var CodeVmAccountDiscriminator = []byte{0xed, 0x82, 0x60, 0x0b, 0xbb, 0x2c, 0xc7, 0x55}

type CodeVmAccount struct {
	Authority    ed25519.PublicKey
	Mint         ed25519.PublicKey
	Omnibus      TokenPool
	LockDuration uint8
	Bump         uint8
	Slot         uint64
	Poh          Hash
	ChangeLog    PagedMemory
}

func (obj *CodeVmAccount) Unmarshal(data []byte) error {
	if len(data) < minCodeVmAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, CodeVmAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Authority, &offset)
	getKey(data, &obj.Mint, &offset)
	getTokenPool(data, &obj.Omnibus, &offset)
	getUint8(data, &obj.LockDuration, &offset)
	getUint8(data, &obj.Bump, &offset)
	getUint64(data, &obj.Slot, &offset)
	getHash(data, &obj.Poh, &offset)
	offset += 5
	obj.ChangeLog = NewChangelogMemory()
	getPagedMemory(data, &obj.ChangeLog, &offset)

	return nil
}

func (obj *CodeVmAccount) String() string {
	return fmt.Sprintf(
		"CodeVmAccount{authority=%s,mint=%s,omnibus=%s,lock_duration=%d,bump=%d,slot=%d,poh=%s,changelog=%s}",
		base58.Encode(obj.Authority),
		base58.Encode(obj.Mint),
		obj.Omnibus.String(),
		obj.LockDuration,
		obj.Bump,
		obj.Slot,
		obj.Poh.String(),
		obj.ChangeLog.String(),
	)
}
