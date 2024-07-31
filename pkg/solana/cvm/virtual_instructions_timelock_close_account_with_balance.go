package cvm

import (
	"crypto/ed25519"

	"github.com/code-payments/code-server/pkg/solana"
	timelock_token "github.com/code-payments/code-server/pkg/solana/timelock/v1"
)

const (
	TimelockCloseAccountWithBalanceVirtrualInstructionDataSize = SignatureSize // signature
)

type TimelockCloseAccountWithBalanceVirtualInstructionArgs struct {
	TimelockBump uint8
	Signature    Signature
}

type TimelockCloseAccountWithBalanceVirtualInstructionAccounts struct {
	VmAuthority          ed25519.PublicKey
	VirtualTimelock      ed25519.PublicKey
	VirtualTimelockVault ed25519.PublicKey
	Destination          ed25519.PublicKey
	Owner                ed25519.PublicKey
	Mint                 ed25519.PublicKey
}

func NewTimelockCloseAccountWithBalanceVirtualInstructionCtor(
	accounts *TimelockCloseAccountWithBalanceVirtualInstructionAccounts,
	args *TimelockCloseAccountWithBalanceVirtualInstructionArgs,
) VirtualInstructionCtor {
	return func() (Opcode, []solana.Instruction, []byte) {
		var offset int
		data := make([]byte, TimelockCloseAccountWithBalanceVirtrualInstructionDataSize)
		putSignature(data, args.Signature, &offset)

		ixns := []solana.Instruction{
			newKreMemoIxn(),
			timelock_token.NewRevokeLockWithAuthorityInstruction(
				&timelock_token.RevokeLockWithAuthorityInstructionAccounts{
					Timelock:      accounts.VirtualTimelock,
					Vault:         accounts.VirtualTimelockVault,
					TimeAuthority: accounts.VmAuthority,
					Payer:         accounts.VmAuthority,
				},
				&timelock_token.RevokeLockWithAuthorityInstructionArgs{
					TimelockBump: args.TimelockBump,
				},
			).ToLegacyInstruction(),
			timelock_token.NewDeactivateInstruction(
				&timelock_token.DeactivateInstructionAccounts{
					Timelock:   accounts.VirtualTimelock,
					VaultOwner: accounts.Owner,
					Payer:      accounts.VmAuthority,
				},
				&timelock_token.DeactivateInstructionArgs{
					TimelockBump: args.TimelockBump,
				},
			).ToLegacyInstruction(),
			timelock_token.NewWithdrawInstruction(
				&timelock_token.WithdrawInstructionAccounts{
					Timelock:    accounts.VirtualTimelock,
					Vault:       accounts.VirtualTimelockVault,
					VaultOwner:  accounts.Owner,
					Destination: accounts.Destination,
					Payer:       accounts.VmAuthority,
				},
				&timelock_token.WithdrawInstructionArgs{
					TimelockBump: args.TimelockBump,
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

		return OpcodeCompoundCloseAccountWithBalance, ixns, data
	}
}
