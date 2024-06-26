package cvm

type VirtualAccountType uint8

const (
	VirtualAccountTypeDurableNonce VirtualAccountType = iota
	VirtualAccountTypeTimelock
	VirtualAccountTypeRelay
)
