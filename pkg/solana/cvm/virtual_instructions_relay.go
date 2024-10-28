package cvm

const (
	RelayVirtrualInstructionDataSize = (8 + // amount
		HashSize + // transcript
		HashSize + // recent_root
		HashSize) // commitment
)

type RelayVirtualInstructionArgs struct {
	Amount     uint64
	Transcript Hash
	RecentRoot Hash
	Commitment Hash
}

func NewRelayVirtualInstruction(
	args *RelayVirtualInstructionArgs,
) VirtualInstruction {
	var offset int
	data := make([]byte, RelayVirtrualInstructionDataSize)

	putUint64(data, args.Amount, &offset)
	putHash(data, args.Transcript, &offset)
	putHash(data, args.RecentRoot, &offset)
	putHash(data, args.Commitment, &offset)

	return VirtualInstruction{
		Opcode: OpcodeRelay,
		Data:   data,
	}
}
