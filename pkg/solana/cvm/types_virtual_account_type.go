package cvm

import "errors"

type VirtualAccountType uint8

const (
	VirtualAccountTypeDurableNonce VirtualAccountType = iota
	VirtualAccountTypeTimelock
	VirtualAccountTypeRelay
)

func GetVirtualAccountTypeFromMemoryLayout(layout MemoryLayout) (VirtualAccountType, error) {
	switch layout {
	case MemoryLayoutNonce:
		return VirtualAccountTypeDurableNonce, nil
	case MemoryLayoutTimelock:
		return VirtualAccountTypeTimelock, nil
	case MemoryLayoutRelay:
		return VirtualAccountTypeRelay, nil
	default:
		return 0, errors.New("memory layout doesn't have a defined virtual account type")
	}
}
