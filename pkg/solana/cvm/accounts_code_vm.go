package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	CodeVmAccountSize = (8 + //discriminator
		32 + // authority
		32 + // mint
		8 + // slot
		HashSize + // poh
		TokenPoolSize + // omnibus
		1 + // lock_duration
		1 + // bump
		5) // padding
)

var CodeVmAccountDiscriminator = []byte{byte(AccountTypeCodeVm), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

type CodeVmAccount struct {
	Authority    ed25519.PublicKey
	Mint         ed25519.PublicKey
	Slot         uint64
	Poh          Hash
	Omnibus      TokenPool
	LockDuration uint8
	Bump         uint8
}

func (obj *CodeVmAccount) Unmarshal(data []byte) error {
	if len(data) < CodeVmAccountSize {
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
	getUint64(data, &obj.Slot, &offset)
	getHash(data, &obj.Poh, &offset)
	getTokenPool(data, &obj.Omnibus, &offset)
	getUint8(data, &obj.LockDuration, &offset)
	getUint8(data, &obj.Bump, &offset)
	offset += 5 // padding

	return nil
}

func (obj *CodeVmAccount) String() string {
	return fmt.Sprintf(
		"CodeVmAccount{authority=%s,mint=%s,slot=%d,poh=%s,omnibus=%s,lock_duration=%d,bump=%d}",
		base58.Encode(obj.Authority),
		base58.Encode(obj.Mint),
		obj.Slot,
		obj.Poh.String(),
		obj.Omnibus.String(),
		obj.LockDuration,
		obj.Bump,
	)
}
