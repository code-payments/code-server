package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	TimelockTransferInternalVirtrualInstructionDataSize = (SignatureSize + // signature
		8) // amount
)

type TimelockTransferInternalVirtualInstructionArgs struct {
	TimelockBump uint8
	Amount       uint64
	Signature    Signature
}

type TimelockTransferInternalVirtualInstructionAccounts struct {
	VmAuthority          ed25519.PublicKey
	VirtualTimelock      ed25519.PublicKey
	VirtualTimelockVault ed25519.PublicKey
	Owner                ed25519.PublicKey
	Destination          ed25519.PublicKey
}

func NewTimelockTransferInternalVirtualInstructionCtor(
	accounts *TimelockTransferInternalVirtualInstructionAccounts,
	args *TimelockTransferInternalVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, TimelockTransferInternalVirtrualInstructionDataSize)
		putSignature(data, args.Signature, &offset)
		putUint64(data, args.Amount, &offset)

		ixns := []solana.Instruction{
			newKreMemoIxn(),
			timelock_token.NewTransferWithAuthorityInstruction(
				&timelock_token.TransferWithAuthorityInstructionAccounts{
					Timelock:      accounts.VirtualTimelock,
					Vault:         accounts.VirtualTimelockVault,
					VaultOwner:    accounts.Owner,
					TimeAuthority: accounts.VmAuthority,
					Destination:   accounts.Destination,
					Payer:         accounts.VmAuthority,
				},
				&timelock_token.TransferWithAuthorityInstructionArgs{
					TimelockBump: args.TimelockBump,
					Amount:       uint64(args.Amount),
				},
			).ToLegacyInstruction(),
		}

		return OpcodeTimelockTransferToInternal, ixns, data
	}
}
