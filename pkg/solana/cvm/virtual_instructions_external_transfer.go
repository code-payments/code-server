package cvm

const (
	ExternalTransferVirtrualInstructionDataSize = (SignatureSize + // signature
		8) // amount
)

type ExternalTransferVirtualInstructionArgs struct {
	Amount    uint64
	Signature Signature
}

func NewExternalTransferVirtualInstruction(
	args *TransferVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, ExternalTransferVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)
	putUint64(data, args.Amount, &offset)

	return VirtualInstruction{
		Opcode: OpcodeExternalTransfer,
		Data:   data,
	}
}
