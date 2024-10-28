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

func putAccountType(dst []byte, v AccountType, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
