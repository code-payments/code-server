package cvm

type Opcode uint8

const (
	OpcodeTimelockTransferInternal Opcode = 36
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
