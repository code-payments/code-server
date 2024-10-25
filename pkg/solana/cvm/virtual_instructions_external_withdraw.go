package cvm

const (
	ExternalWithdrawVirtrualInstructionDataSize = SignatureSize // signature
)

type ExternalWithdrawVirtualInstructionArgs struct {
	Signature Signature
}

func NewExternalWithdrawVirtualInstruction(
	args *ExternalWithdrawVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, ExternalWithdrawVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)

	return VirtualInstruction{
		Opcode: OpcodeExternalWithdraw,
		Data:   data,
	}
}
