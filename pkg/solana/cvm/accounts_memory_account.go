package cvm

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	MaxMemoryAccountNameLength = 32
)

type MemoryAccountWithData struct {
	Vm     ed25519.PublicKey
	Bump   uint8
	Name   string
	Layout MemoryLayout
	Data   PagedMemory
}

const MemoryAccountWithDataSize = (8 + // discriminator
	32 + // vm
	1 + // bump
	MaxMemoryAccountNameLength + // name
	1 + // layout
	PagedMemorySize) // data

var MemoryAccountDiscriminator = []byte{0x89, 0x7a, 0xdc, 0x6e, 0xdd, 0xca, 0x3e, 0x7f}

func (obj *MemoryAccountWithData) Unmarshal(data []byte) error {
	if len(data) < MemoryAccountWithDataSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, MemoryAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getUint8(data, &obj.Bump, &offset)
	getFixedString(data, &obj.Name, MaxMemoryAccountNameLength, &offset)
	getMemoryLayout(data, &obj.Layout, &offset)
	getPagedMemory(data, &obj.Data, &offset)

	return nil
}

func (obj *MemoryAccountWithData) String() string {
	return fmt.Sprintf(
		"MemoryAccountWithData{vm=%s,bump=%d,name=%s,data=%s}",
		base58.Encode(obj.Vm),
		obj.Bump,
		obj.Name,
		obj.Data.String(),
	)
}
