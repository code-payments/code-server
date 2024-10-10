package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	TimelockTransferRelayVirtrualInstructionDataSize = (SignatureSize + // signature
		8) // amount
)

type TimelockTransferRelayVirtualInstructionArgs struct {
	TimelockBump uint8
	Amount       uint64
	Signature    Signature
}

type TimelockTransferRelayVirtualInstructionAccounts struct {
	VmAuthority          ed25519.PublicKey
	VirtualTimelock      ed25519.PublicKey
	VirtualTimelockVault ed25519.PublicKey
	Owner                ed25519.PublicKey
	RelayVault           ed25519.PublicKey
}

func NewTimelockTransferRelayVirtualInstructionCtor(
	accounts *TimelockTransferRelayVirtualInstructionAccounts,
	args *TimelockTransferRelayVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, TimelockTransferRelayVirtrualInstructionDataSize)
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
					Destination:   accounts.RelayVault,
					Payer:         accounts.VmAuthority,
				},
				&timelock_token.TransferWithAuthorityInstructionArgs{
					TimelockBump: args.TimelockBump,
					Amount:       uint64(args.Amount),
				},
			).ToLegacyInstruction(),
		}

		return OpcodeTimelockTransferToRelay, ixns, data
	}
}
