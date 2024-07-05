package cvm

import (
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	RelayTransferInternalVirtrualInstructionDataSize = (8 + // amount
		HashSize + // transcript
		HashSize + // recent_root
		HashSize) // commitment
)

type RelayTransferInternalVirtualInstructionArgs struct {
	Amount     uint64
	Transcript Hash
	RecentRoot Hash
	Commitment Hash
}

type RelayTransferInternalVirtualInstructionAccounts struct {
}

func NewRelayTransferInternalVirtualInstructionCtor(
	accounts *RelayTransferInternalVirtualInstructionAccounts,
	args *RelayTransferInternalVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, RelayTransferInternalVirtrualInstructionDataSize)

		putUint64(data, args.Amount, &offset)
		putHash(data, args.Transcript, &offset)
		putHash(data, args.RecentRoot, &offset)
		putHash(data, args.Commitment, &offset)

		return OpcodeTransferWithCommitmentInternal, nil, data
	}
}
