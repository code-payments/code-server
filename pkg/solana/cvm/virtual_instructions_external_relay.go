package cvm

const (
	ExternalRelayVirtrualInstructionDataSize = (8 + // amount
		HashSize + // transcript
		HashSize + // recent_root
		HashSize) // commitment
)

type ExternalRelayVirtualInstructionArgs struct {
	Amount     uint64
	Transcript Hash
	RecentRoot Hash
	Commitment Hash
}

func NewExternalRelayVirtualInstruction(
	args *ExternalRelayVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, ExternalRelayVirtrualInstructionDataSize)

	putUint64(data, args.Amount, &offset)
	putHash(data, args.Transcript, &offset)
	putHash(data, args.RecentRoot, &offset)
	putHash(data, args.Commitment, &offset)

	return VirtualInstruction{
		Opcode: OpcodeExternalRelay,
		Data:   data,
	}
}
