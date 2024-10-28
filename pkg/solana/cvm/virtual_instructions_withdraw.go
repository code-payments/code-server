package cvm

const (
	WithdrawVirtrualInstructionDataSize = SignatureSize // signature
)

type WithdrawVirtualInstructionArgs struct {
	Signature Signature
}

func NewWithdrawVirtualInstruction(
	args *WithdrawVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, WithdrawVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)

	return VirtualInstruction{
		Opcode: OpcodeWithdraw,
		Data:   data,
	}
}
