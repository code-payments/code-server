package cvm

const (
	AirdropVirtrualInstructionDataSize = (SignatureSize + // signature
		8 + // amount
		1) // count
)

type AirdropVirtualInstructionArgs struct {
	Amount    uint64
	Count     uint8
	Signature Signature
}

func NewAirdropVirtualInstruction(
	args *AirdropVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, AirdropVirtrualInstructionDataSize)
	putSignature(data, args.Signature, &offset)
	putUint64(data, args.Amount, &offset)
	putUint8(data, args.Count, &offset)

	return VirtualInstruction{
		Opcode: OpcodeAirdrop,
		Data:   data,
	}
}
