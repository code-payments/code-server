package cvm

const (
	TransferVirtrualInstructionDataSize = (SignatureSize + // signature
		8) // amount
)

type TransferVirtualInstructionArgs struct {
	Amount    uint64
	Signature Signature
}

func NewTransferVirtualInstruction(
	args *TransferVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, TransferVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)
	putUint64(data, args.Amount, &offset)

	return VirtualInstruction{
		Opcode: OpcodeTransfer,
		Data:   data,
	}
}
