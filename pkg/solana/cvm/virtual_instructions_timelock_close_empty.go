package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	TimelockCloseEmptyVirtrualInstructionDataSize = (SignatureSize + // signature
		4) // max_amount
)

type TimelockCloseEmptyVirtualInstructionArgs struct {
	TimelockBump uint8
	MaxAmount    uint32
	Signature    Signature
}

type TimelockCloseEmptyVirtualInstructionAccounts struct {
	VmAuthority          ed25519.PublicKey
	VirtualTimelock      ed25519.PublicKey
	VirtualTimelockVault ed25519.PublicKey
	Owner                ed25519.PublicKey
	Mint                 ed25519.PublicKey
}

func NewTimelockCloseEmptyVirtualInstructionCtor(
	accounts *TimelockCloseEmptyVirtualInstructionAccounts,
	args *TimelockCloseEmptyVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, TimelockCloseEmptyVirtrualInstructionDataSize)
		putSignature(data, args.Signature, &offset)
		putUint32(data, args.MaxAmount, &offset)

		ixns := []solana.Instruction{
			timelock_token.NewBurnDustWithAuthorityInstruction(
				&timelock_token.BurnDustWithAuthorityInstructionAccounts{
					Timelock:      accounts.VirtualTimelock,
					Vault:         accounts.VirtualTimelockVault,
					VaultOwner:    accounts.Owner,
					TimeAuthority: accounts.VmAuthority,
					Mint:          accounts.Mint,
					Payer:         accounts.VmAuthority,
				},
				&timelock_token.BurnDustWithAuthorityInstructionArgs{
					TimelockBump: args.TimelockBump,
					MaxAmount:    uint64(args.MaxAmount),
				},
			).ToLegacyInstruction(),
			timelock_token.NewCloseAccountsInstruction(
				&timelock_token.CloseAccountsInstructionAccounts{
					Timelock:       accounts.VirtualTimelock,
					Vault:          accounts.VirtualTimelockVault,
					CloseAuthority: accounts.VmAuthority,
					Payer:          accounts.VmAuthority,
				},
				&timelock_token.CloseAccountsInstructionArgs{
					TimelockBump: args.TimelockBump,
				},
			).ToLegacyInstruction(),
		}

		return OpcodeCompoundCloseEmptyAccount, ixns, data
	}
}
