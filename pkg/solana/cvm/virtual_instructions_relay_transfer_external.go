package cvm

import (
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	RelayTransferExternalVirtrualInstructionDataSize = (8 + // amount
		HashSize + // transcript
		HashSize + // recent_root
		HashSize) // commitment
)

type RelayTransferExternalVirtualInstructionArgs struct {
	Amount     uint64
	Transcript Hash
	RecentRoot Hash
	Commitment Hash
}

type RelayTransferExternalVirtualInstructionAccounts struct {
}

func NewRelayTransferExternalVirtualInstructionCtor(
	accounts *RelayTransferExternalVirtualInstructionAccounts,
	args *RelayTransferExternalVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, RelayTransferExternalVirtrualInstructionDataSize)

		putUint64(data, args.Amount, &offset)
		putHash(data, args.Transcript, &offset)
		putHash(data, args.RecentRoot, &offset)
		putHash(data, args.Commitment, &offset)

		return OpcodeTransferWithCommitmentExternal, nil, data
	}
}
