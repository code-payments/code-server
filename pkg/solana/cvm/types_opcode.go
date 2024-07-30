package cvm

type Opcode uint8

const (
	OpcodeTimelockTransferInternal Opcode = 36
	OpcodeTimelockTransferExternal Opcode = 37
	OpcodeTimelockTransferRelay    Opcode = 38

	OpcodeTransferWithCommitmentInternal Opcode = 52
	OpcodeTransferWithCommitmentExternal Opcode = 53

	OpcodeCompoundCloseEmptyAccount       Opcode = 60
	OpcodeCompoundCloseAccountWithBalance Opcode = 61
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
