package cvm

type AccountType uint8

const (
	AccountTypeUnknown AccountType = iota
	AccountTypeCodeVm
	AccountTypeMemory
	AccountTypeStorage
	AccountTypeRelay
	AccountTypeUnlockState
	AccountTypeWithdrawReceipt
)
