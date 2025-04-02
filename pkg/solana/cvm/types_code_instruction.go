package cvm

type CodeInstruction uint8

const (
	Unknown CodeInstruction = iota

	CodeInstructionInitVm
	CodeInstructionInitMemory
	CodeInstructionInitStorage
	CodeInstructionInitRelay
	CodeInstructionInitNonce
	CodeInstructionInitTimelock
	CodeInstructionInitUnlock

	CodeInstructionExec
	CodeInstructionCompress
	CodeInstructionDecompress

	CodeInstructionResizeMemory
	CodeInstructionSnapshot

	CodeInstructionDeposit
	CodeInstructionWithdraw
	CodeInstructionUnlockI
)

func putCodeInstruction(dst []byte, v CodeInstruction, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
