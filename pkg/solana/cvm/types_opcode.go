package cvm

type Opcode uint8

const (
	OpcodeTimelockTransferInternal Opcode = 36
	OpcodeTimelockTransferExternal Opcode = 37

	OpcodeTransferWithCommitmentInternal Opcode = 52
	OpcodeTransferWithCommitmentExternal Opcode = 53
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
