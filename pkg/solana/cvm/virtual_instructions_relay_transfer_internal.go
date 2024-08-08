package cvm

import (
	"github.com/code-payments/code-server/pkg/solana"
)

const (
	RelayTransferInternalVirtrualInstructionDataSize = (4 + // amount
		HashSize + // transcript
		HashSize + // recent_root
		HashSize) // commitment
)

type RelayTransferInternalVirtualInstructionArgs struct {
	Amount     uint32
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

		putUint32(data, args.Amount, &offset)
		putHash(data, args.Transcript, &offset)
		putHash(data, args.RecentRoot, &offset)
		putHash(data, args.Commitment, &offset)

		return OpcodeSplitterTransferToInternal, nil, data
	}
}
