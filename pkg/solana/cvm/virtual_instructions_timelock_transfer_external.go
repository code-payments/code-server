package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	TimelockTransferExternalVirtrualInstructionDataSize = 8 // amount
)

type TimelockTransferExternalVirtualInstructionArgs struct {
	TimelockBump uint8
	Amount       uint64
}

type TimelockTransferExternalVirtualInstructionAccounts struct {
	VmAuthority     ed25519.PublicKey
	VirtualTimelock ed25519.PublicKey
	VirtualVault    ed25519.PublicKey
	Owner           ed25519.PublicKey
	Destination     ed25519.PublicKey
}

func NewTimelockTransferExternalVirtualInstructionCtor(
	accounts *TimelockTransferExternalVirtualInstructionAccounts,
	args *TimelockTransferExternalVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, TimelockTransferExternalVirtrualInstructionDataSize)
		putUint64(data, args.Amount, &offset)

		ixns := []solana.Instruction{
			newKreMemoIxn(),
			timelock_token.NewTransferWithAuthorityInstruction(
				&timelock_token.TransferWithAuthorityInstructionAccounts{
					Timelock:      accounts.VirtualTimelock,
					Vault:         accounts.VirtualVault,
					VaultOwner:    accounts.Owner,
					TimeAuthority: accounts.VmAuthority,
					Destination:   accounts.Destination,
					Payer:         accounts.VmAuthority,
				},
				&timelock_token.TransferWithAuthorityInstructionArgs{
					TimelockBump: args.TimelockBump,
					Amount:       args.Amount,
				},
			).ToLegacyInstruction(),
		}

		return OpcodeTimelockTransferExternal, ixns, data
	}
}
