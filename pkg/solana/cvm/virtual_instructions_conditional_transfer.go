package cvm

const (
	ConditionalTransferVirtrualInstructionDataSize = (SignatureSize + // signature
		8) // amount
)

type ConditionalTransferVirtualInstructionArgs struct {
	Amount    uint64
	Signature Signature
}

func NewConditionalTransferVirtualInstruction(
	args *ConditionalTransferVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, ConditionalTransferVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)
	putUint64(data, args.Amount, &offset)

	return VirtualInstruction{
		Opcode: OpcodeConditionalTransfer,
		Data:   data,
	}
}
