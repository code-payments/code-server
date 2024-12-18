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
	Vm          ed25519.PublicKey
	Name        string
	Bump        uint8
	Version     MemoryVersion
	AccountSize uint16
	NumAccounts uint32
}

type MemoryAccountWithData struct {
	Vm          ed25519.PublicKey
	Name        string
	Bump        uint8
	Version     MemoryVersion
	AccountSize uint16
	NumAccounts uint32
	Data        SliceAllocator // todo: support other implementations
}

const MemoryAccountSize = (8 + // discriminator
	32 + // vm
	MaxMemoryAccountNameLength + // name
	1 + // bump
	1 + // version
	2 + // account_size
	4) // num_accounts

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
	getMemoryVersion(data, &obj.Version, &offset)

	switch obj.Version {
	case MemoryVersionV0:
		var layout MemoryLayout
		offset += 5 // padding
		getMemoryLayout(data, &layout, &offset)

		switch layout {
		case MemoryLayoutTimelock:
			obj.AccountSize = uint16(GetVirtualAccountSizeInMemory(VirtualAccountTypeTimelock))
		case MemoryLayoutNonce:
			obj.AccountSize = uint16(GetVirtualAccountSizeInMemory(VirtualAccountTypeDurableNonce))
		case MemoryLayoutRelay:
			obj.AccountSize = uint16(GetVirtualAccountSizeInMemory(VirtualAccountTypeRelay))
		default:
			return errors.New("unsupported memory layout")
		}
		obj.NumAccounts = MemoryV0NumAccounts

	case MemoryVersionV1:
		getUint16(data, &obj.AccountSize, &offset)
		getUint32(data, &obj.NumAccounts, &offset)

	default:
		return errors.New("invalid memory account version")
	}

	return nil
}

func (obj *MemoryAccount) String() string {
	return fmt.Sprintf(
		"MemoryAccount{vm=%s,bump=%d,name=%s,version=%d,num_accounts=%d,account_size=%d}",
		base58.Encode(obj.Vm),
		obj.Bump,
		obj.Name,
		obj.Version,
		obj.NumAccounts,
		obj.AccountSize,
	)
}

func (obj *MemoryAccountWithData) Unmarshal(data []byte) error {
	var memoryAccount MemoryAccount
	if err := memoryAccount.Unmarshal(data); err != nil {
		return err
	}

	obj.Vm = memoryAccount.Vm
	obj.Name = memoryAccount.Name
	obj.Bump = memoryAccount.Bump
	obj.Version = memoryAccount.Version
	obj.AccountSize = memoryAccount.AccountSize
	obj.NumAccounts = memoryAccount.NumAccounts

	if len(data) < MemoryAccountSize+GetSliceAllocatorSize(int(obj.NumAccounts), int(obj.AccountSize)) {
		return ErrInvalidAccountData
	}

	offset := MemoryAccountSize
	getSliceAllocator(data, &obj.Data, int(obj.NumAccounts), int(obj.AccountSize), &offset)

	return nil
}

func (obj *MemoryAccountWithData) String() string {
	return fmt.Sprintf(
		"MemoryAccountWithData{vm=%s,name=%s,bump=%d,version=%d,num_accounts=%d,account_size=%d,data=%s}",
		base58.Encode(obj.Vm),
		obj.Name,
		obj.Bump,
		obj.Version,
		obj.NumAccounts,
		obj.AccountSize,
		obj.Data.String(),
	)
}
