package cvm

type Opcode uint8

const (
	OpcodeTimelockTransferToInternal Opcode = 10
	OpcodeTimelockTransferToExternal Opcode = 11
	OpcodeTimelockTransferToRelay    Opcode = 12
	OpcodeTimelockWithdrawToInternal Opcode = 13
	OpcodeTimelockWithdrawToExternal Opcode = 14

	OpcodeSplitterTransferToInternal Opcode = 20
	OpcodeSplitterTransferToExternal Opcode = 21
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
