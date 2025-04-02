package cvm

type Opcode uint8

const (
	OpcodeUnknown Opcode = 0

	OpcodeTransfer Opcode = 11
	OpcodeWithdraw Opcode = 14
	OpcodeRelay    Opcode = 21

	OpcodeExternalTransfer Opcode = 10
	OpcodeExternalWithdraw Opcode = 13
	OpcodeExternalRelay    Opcode = 20

	OpcodeConditionalTransfer Opcode = 12

	OpcodeAirdrop Opcode = 30
)

func putOpcode(dst []byte, v Opcode, offset *int) {
	dst[*offset] = uint8(v)
	*offset += 1
}
