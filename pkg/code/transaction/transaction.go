package transaction

import (
	"crypto/ed25519"
	"errors"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

// todo: The argument sizes are blowing out of proportion, though there's likely
//       a larger refactor going to happen anyways when we support batching of
//       many virtual instructions into a single Solana transaction.

// todo: Support external variation of transfers

var (
	// Should be equal to minimum bucket size
	maxBurnAmount = kin.ToQuarks(1)
)

// MakeNoncedTransaction makes a transaction that's backed by a nonce. The returned
// transaction is not signed.
func MakeNoncedTransaction(nonce *common.Account, bh solana.Blockhash, instructions ...solana.Instruction) (solana.Transaction, error) {
	if len(instructions) == 0 {
		return solana.Transaction{}, errors.New("no instructions provided")
	}

	advanceNonceInstruction, err := makeAdvanceNonceInstruction(nonce)
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions = append([]solana.Instruction{advanceNonceInstruction}, instructions...)

	txn := solana.NewTransaction(common.GetSubsidizer().PublicKey().ToBytes(), instructions...)
	txn.SetBlockhash(bh)

	return txn, nil
}

func MakeOpenAccountTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	memory *common.Account,
	accountIndex int,

	timelockAccounts *common.TimelockAccounts,
) (solana.Transaction, error) {
	initializeInstruction, err := timelockAccounts.GetInitializeInstruction(memory, uint16(accountIndex))
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := []solana.Instruction{
		initializeInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}

func MakeCloseEmptyAccountTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,
	virtualNonce *common.Account,
	virtualBlockhash solana.Blockhash,

	vm *common.Account,
	nonceMemory *common.Account,
	nonceIndex uint16,
	accountMemory *common.Account,
	accountIndex uint16,

	timelockAccounts *common.TimelockAccounts,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(accountMemory.PublicKey().ToBytes())
	unlockPdaBytes := ed25519.PublicKey(timelockAccounts.Unlock.PublicKey().ToBytes())

	closeEmptyAccountVirtualIxn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockCloseEmptyVirtualInstructionCtor(
			&cvm.TimelockCloseEmptyVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      timelockAccounts.State.PublicKey().ToBytes(),
				VirtualTimelockVault: timelockAccounts.Vault.PublicKey().ToBytes(),
				Owner:                timelockAccounts.VaultOwner.PublicKey().ToBytes(),
				Mint:                 timelockAccounts.Mint.PublicKey().ToBytes(),
			},
			&cvm.TimelockCloseEmptyVirtualInstructionArgs{
				TimelockBump: timelockAccounts.StateBump,
				MaxAmount:    uint32(maxBurnAmount),
				Signature:    cvm.Signature(virtualSignature),
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemA:      &memoryAPublicKeyBytes,
			VmMemB:      &memoryBPublicKeyBytes,
			VmUnlockPda: &unlockPdaBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     closeEmptyAccountVirtualIxn.Opcode,
			MemIndices: []uint16{nonceIndex, accountIndex},
			MemBanks:   []uint8{0, 1},
			Data:       closeEmptyAccountVirtualIxn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeInternalCloseAccountWithBalanceTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,
	virtualNonce *common.Account,
	virtualBlockhash solana.Blockhash,

	vm *common.Account,
	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,
	destinationMemory *common.Account,
	destinationIndex uint16,

	source *common.TimelockAccounts,
	destination *common.Account,

	additionalMemo *string,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())
	memoryCPublicKeyBytes := ed25519.PublicKey(destinationMemory.PublicKey().ToBytes())
	unlockPdaBytes := ed25519.PublicKey(source.Unlock.PublicKey().ToBytes())

	transferVirtualIxn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockCloseAccountWithBalanceVirtualInstructionCtor(
			&cvm.TimelockCloseAccountWithBalanceVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      source.State.PublicKey().ToBytes(),
				VirtualTimelockVault: source.Vault.PublicKey().ToBytes(),
				Owner:                source.VaultOwner.PublicKey().ToBytes(),
				Destination:          destination.PublicKey().ToBytes(),
				Mint:                 source.Mint.PublicKey().ToBytes(),
			},
			&cvm.TimelockCloseAccountWithBalanceVirtualInstructionArgs{
				TimelockBump: source.StateBump,
				Signature:    cvm.Signature(virtualSignature),
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemA:      &memoryAPublicKeyBytes,
			VmMemB:      &memoryBPublicKeyBytes,
			VmMemC:      &memoryCPublicKeyBytes,
			VmUnlockPda: &unlockPdaBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     transferVirtualIxn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   []uint8{0, 1, 2},
			Data:       transferVirtualIxn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeInternalTransferWithAuthorityTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,
	virtualNonce *common.Account,
	virtualBlockhash solana.Blockhash,

	vm *common.Account,
	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,
	destinationMemory *common.Account,
	destinationIndex uint16,

	source *common.TimelockAccounts,
	destination *common.Account,
	kinAmountInQuarks uint32,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())
	memoryCPublicKeyBytes := ed25519.PublicKey(destinationMemory.PublicKey().ToBytes())
	unlockPdaBytes := ed25519.PublicKey(source.Unlock.PublicKey().ToBytes())

	transferVirtualIxn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockTransferInternalVirtualInstructionCtor(
			&cvm.TimelockTransferInternalVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      source.State.PublicKey().ToBytes(),
				VirtualTimelockVault: source.Vault.PublicKey().ToBytes(),
				Owner:                source.VaultOwner.PublicKey().ToBytes(),
				Destination:          destination.PublicKey().ToBytes(),
			},
			&cvm.TimelockTransferInternalVirtualInstructionArgs{
				TimelockBump: source.StateBump,
				Amount:       kinAmountInQuarks,
				Signature:    cvm.Signature(virtualSignature),
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemA:      &memoryAPublicKeyBytes,
			VmMemB:      &memoryBPublicKeyBytes,
			VmMemC:      &memoryCPublicKeyBytes,
			VmUnlockPda: &unlockPdaBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     transferVirtualIxn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   []uint8{0, 1, 2},
			Data:       transferVirtualIxn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeInternalTreasuryAdvanceTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	vm *common.Account,
	accountMemory *common.Account,
	accountIndex uint16,
	relayMemory *common.Account,
	relayIndex uint16,

	treasuryPool *common.Account,
	treasuryPoolVault *common.Account,
	destination *common.Account,
	commitment *common.Account,
	kinAmountInQuarks uint32,
	transcript []byte,
	recentRoot []byte,
) (solana.Transaction, error) {
	relayPublicKeyBytes := ed25519.PublicKey(treasuryPool.PublicKey().ToBytes())
	relayVaultPublicKeyBytes := ed25519.PublicKey(treasuryPoolVault.PublicKey().ToBytes())
	memoryAPublicKeyBytes := ed25519.PublicKey(accountMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(relayMemory.PublicKey().ToBytes())

	relayTransferInternalVirtualInstruction := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		nil,
		cvm.NewRelayTransferInternalVirtualInstructionCtor(
			&cvm.RelayTransferInternalVirtualInstructionAccounts{},
			&cvm.RelayTransferInternalVirtualInstructionArgs{
				Transcript: cvm.Hash(transcript),
				RecentRoot: cvm.Hash(recentRoot),
				Commitment: cvm.Hash(commitment.PublicKey().ToBytes()),
				Amount:     kinAmountInQuarks,
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority:  common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:           vm.PublicKey().ToBytes(),
			VmMemA:       &memoryAPublicKeyBytes,
			VmOmnibus:    &memoryBPublicKeyBytes,
			VmRelay:      &relayPublicKeyBytes,
			VmRelayVault: &relayVaultPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     relayTransferInternalVirtualInstruction.Opcode,
			MemIndices: []uint16{accountIndex, relayIndex},
			MemBanks:   []uint8{0, 1},
			Data:       relayTransferInternalVirtualInstruction.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}
