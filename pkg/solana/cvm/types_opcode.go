package cvm

type Opcode uint8

const (
	OpcodeTimelockTransferToExternal Opcode = 10
	OpcodeTimelockTransferToInternal Opcode = 11
	OpcodeTimelockTransferToRelay    Opcode = 12
	OpcodeTimelockWithdrawToExternal Opcode = 13
	OpcodeTimelockWithdrawToInternal Opcode = 14

	OpcodeSplitterTransferToExternal Opcode = 20
	OpcodeSplitterTransferToInternal Opcode = 21
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
