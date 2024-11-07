package cvm

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/mr-tron/base58"
)

const (
	MaxMemoryAccountNameLength = 32
)

type MemoryAccount struct {
	Vm     ed25519.PublicKey
	Name   string
	Bump   uint8
	Layout MemoryLayout
}

type MemoryAccountWithData struct {
	Vm     ed25519.PublicKey
	Name   string
	Bump   uint8
	Layout MemoryLayout
	Data   SimpleMemoryAllocator // todo: support other implementations
}

const MemoryAccountSize = (8 + // discriminator
	32 + // vm
	MaxMemoryAccountNameLength + // name
	1 + // bump
	6 + // padding
	1) // layout

var MemoryAccountDiscriminator = []byte{byte(AccountTypeMemory), 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

func (obj *MemoryAccount) Unmarshal(data []byte) error {
	if len(data) < MemoryAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, MemoryAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getFixedString(data, &obj.Name, MaxMemoryAccountNameLength, &offset)
	getUint8(data, &obj.Bump, &offset)
	offset += 6 // padding
	getMemoryLayout(data, &obj.Layout, &offset)

	return nil
}

func (obj *MemoryAccount) String() string {
	return fmt.Sprintf(
		"MemoryAccount{vm=%s,bump=%d,name=%s,layout=%d}",
		base58.Encode(obj.Vm),
		obj.Bump,
		obj.Name,
		obj.Layout,
	)
}

func (obj *MemoryAccountWithData) Unmarshal(data []byte) error {
	if len(data) < MemoryAccountSize {
		return ErrInvalidAccountData
	}

	var offset int

	var discriminator []byte
	getDiscriminator(data, &discriminator, &offset)
	if !bytes.Equal(discriminator, MemoryAccountDiscriminator) {
		return ErrInvalidAccountData
	}

	getKey(data, &obj.Vm, &offset)
	getFixedString(data, &obj.Name, MaxMemoryAccountNameLength, &offset)
	getUint8(data, &obj.Bump, &offset)
	getMemoryLayout(data, &obj.Layout, &offset)
	offset += 6 // padding

	switch obj.Layout {
	case MemoryLayoutTimelock:
		capacity := CompactStateItems
		itemSize := int(GetVirtualAccountSizeInMemory(VirtualAccountTypeTimelock))
		if len(data) < MemoryAccountSize+GetSimpleMemoryAllocatorSize(capacity, itemSize) {
			return ErrInvalidAccountData
		}
		getSimpleMemoryAllocator(data, &obj.Data, capacity, itemSize, &offset)
	case MemoryLayoutNonce:
		capacity := CompactStateItems
		itemSize := int(GetVirtualAccountSizeInMemory(VirtualAccountTypeDurableNonce))
		if len(data) < MemoryAccountSize+GetSimpleMemoryAllocatorSize(capacity, itemSize) {
			return ErrInvalidAccountData
		}
		getSimpleMemoryAllocator(data, &obj.Data, capacity, itemSize, &offset)
	case MemoryLayoutRelay:
		capacity := CompactStateItems
		itemSize := int(GetVirtualAccountSizeInMemory(VirtualAccountTypeRelay))
		if len(data) < MemoryAccountSize+GetSimpleMemoryAllocatorSize(capacity, itemSize) {
			return ErrInvalidAccountData
		}
		getSimpleMemoryAllocator(data, &obj.Data, capacity, itemSize, &offset)
	default:
		return errors.New("unsupported memory layout")
	}

	return nil
}

func (obj *MemoryAccountWithData) String() string {
	return fmt.Sprintf(
		"MemoryAccountWithData{vm=%s,name=%s,bump=%d,layout=%d,data=%s}",
		base58.Encode(obj.Vm),
		obj.Name,
		obj.Bump,
		obj.Layout,
		obj.Data.String(),
	)
}
