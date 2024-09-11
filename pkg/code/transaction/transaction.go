package transaction

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

// todo: The argument sizes are blowing out of proportion, though there's likely
//       a larger refactor going to happen anyways when we support batching of
//       many virtual instructions into a single Solana transaction.

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
	accountIndex uint16,

	timelockAccounts *common.TimelockAccounts,
) (solana.Transaction, error) {
	initializeInstruction, err := timelockAccounts.GetInitializeInstruction(memory, accountIndex)
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := []solana.Instruction{
		initializeInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}

func MakeCompressAccountTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	vm *common.Account,
	memory *common.Account,
	accountIndex uint16,
	storage *common.Account,
	virtualAccountState []byte,
) (solana.Transaction, error) {
	hasher := sha256.New()
	hasher.Write(virtualAccountState)
	hashedVirtualAccountState := hasher.Sum(nil)

	signature := ed25519.Sign(common.GetSubsidizer().PrivateKey().ToBytes(), hashedVirtualAccountState)

	compressInstruction := cvm.NewSystemAccountCompressInstruction(
		&cvm.SystemAccountCompressInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemory:    memory.PublicKey().ToBytes(),
			VmStorage:   storage.PublicKey().ToBytes(),
		},
		&cvm.SystemAccountCompressInstructionArgs{
			AccountIndex: accountIndex,
			Signature:    cvm.Signature(signature),
		},
	)

	return MakeNoncedTransaction(nonce, bh, compressInstruction)
}

func MakeInternalWithdrawTransaction(
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
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())
	memoryCPublicKeyBytes := ed25519.PublicKey(destinationMemory.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockWithdrawInternalVirtualInstructionCtor(
			&cvm.TimelockWithdrawInternalVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      source.State.PublicKey().ToBytes(),
				VirtualTimelockVault: source.Vault.PublicKey().ToBytes(),
				Owner:                source.VaultOwner.PublicKey().ToBytes(),
				Destination:          destination.PublicKey().ToBytes(),
				Mint:                 source.Mint.PublicKey().ToBytes(),
			},
			&cvm.TimelockWithdrawInternalVirtualInstructionArgs{
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
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   []uint8{0, 1, 2},
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeExternalWithdrawTransaction(
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

	source *common.TimelockAccounts,
	destination *common.Account,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())

	destinationPublicKeyBytes := ed25519.PublicKey(destination.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockWithdrawExternalVirtualInstructionCtor(
			&cvm.TimelockWithdrawExternalVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      source.State.PublicKey().ToBytes(),
				VirtualTimelockVault: source.Vault.PublicKey().ToBytes(),
				Owner:                source.VaultOwner.PublicKey().ToBytes(),
				Destination:          destination.PublicKey().ToBytes(),
				Mint:                 source.Mint.PublicKey().ToBytes(),
			},
			&cvm.TimelockWithdrawExternalVirtualInstructionArgs{
				TimelockBump: source.StateBump,
				Signature:    cvm.Signature(virtualSignature),
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          &memoryAPublicKeyBytes,
			VmMemB:          &memoryBPublicKeyBytes,
			ExternalAddress: &destinationPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex},
			MemBanks:   []uint8{0, 1},
			Data:       vixn.Data,
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
	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())
	memoryCPublicKeyBytes := ed25519.PublicKey(destinationMemory.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
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
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   []uint8{0, 1, 2},
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeExternalTransferWithAuthorityTransaction(
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

	source *common.TimelockAccounts,
	destination *common.Account,
	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())

	destinationPublicKeyBytes := ed25519.PublicKey(destination.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
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
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          &memoryAPublicKeyBytes,
			VmMemB:          &memoryBPublicKeyBytes,
			ExternalAddress: &destinationPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex},
			MemBanks:   []uint8{0, 1},
			Data:       vixn.Data,
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
	kinAmountInQuarks uint64,
	transcript []byte,
	recentRoot []byte,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(accountMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(relayMemory.PublicKey().ToBytes())

	treasuryPoolPublicKeyBytes := ed25519.PublicKey(treasuryPool.PublicKey().ToBytes())
	treasuryPoolVaultPublicKeyBytes := ed25519.PublicKey(treasuryPoolVault.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
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
			VmMemB:       &memoryBPublicKeyBytes,
			VmRelay:      &treasuryPoolPublicKeyBytes,
			VmRelayVault: &treasuryPoolVaultPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{accountIndex, relayIndex},
			MemBanks:   []uint8{0, 1},
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeExternalTreasuryAdvanceTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	vm *common.Account,
	relayMemory *common.Account,
	relayIndex uint16,

	treasuryPool *common.Account,
	treasuryPoolVault *common.Account,
	destination *common.Account,
	commitment *common.Account,
	kinAmountInQuarks uint64,
	transcript []byte,
	recentRoot []byte,
) (solana.Transaction, error) {
	memoryAPublicKeyBytes := ed25519.PublicKey(relayMemory.PublicKey().ToBytes())

	treasuryPoolPublicKeyBytes := ed25519.PublicKey(treasuryPool.PublicKey().ToBytes())
	treasuryPoolVaultPublicKeyBytes := ed25519.PublicKey(treasuryPoolVault.PublicKey().ToBytes())

	destinationPublicKeyBytes := ed25519.PublicKey(destination.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		nil,
		cvm.NewRelayTransferExternalVirtualInstructionCtor(
			&cvm.RelayTransferExternalVirtualInstructionAccounts{},
			&cvm.RelayTransferExternalVirtualInstructionArgs{
				Transcript: cvm.Hash(transcript),
				RecentRoot: cvm.Hash(recentRoot),
				Commitment: cvm.Hash(commitment.PublicKey().ToBytes()),
				Amount:     kinAmountInQuarks,
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          &memoryAPublicKeyBytes,
			VmRelay:         &treasuryPoolPublicKeyBytes,
			VmRelayVault:    &treasuryPoolVaultPublicKeyBytes,
			ExternalAddress: &destinationPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{relayIndex},
			MemBanks:   []uint8{0},
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeCashChequeTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,
	virtualNonce *common.Account,
	virtualBlockhash solana.Blockhash,

	vm *common.Account,
	vmOmnibus *common.Account,

	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,
	relayMemory *common.Account,
	relayIndex uint16,

	source *common.TimelockAccounts,
	treasuryPool *common.Account,
	treasuryPoolVault *common.Account,
	commitmentVault *common.Account,
	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	vmOmnibusPublicKeyBytes := ed25519.PublicKey(vmOmnibus.PublicKey().ToBytes())

	memoryAPublicKeyBytes := ed25519.PublicKey(nonceMemory.PublicKey().ToBytes())
	memoryBPublicKeyBytes := ed25519.PublicKey(sourceMemory.PublicKey().ToBytes())
	memoryCPublicKeyBytes := ed25519.PublicKey(relayMemory.PublicKey().ToBytes())

	treasuryPoolPublicKeyBytes := ed25519.PublicKey(treasuryPool.PublicKey().ToBytes())
	treasuryPoolVaultPublicKeyBytes := ed25519.PublicKey(treasuryPoolVault.PublicKey().ToBytes())

	vixn := cvm.NewVirtualInstruction(
		common.GetSubsidizer().PublicKey().ToBytes(),
		&cvm.VirtualDurableNonce{
			Address: virtualNonce.PublicKey().ToBytes(),
			Nonce:   cvm.Hash(virtualBlockhash),
		},
		cvm.NewTimelockTransferRelayVirtualInstructionCtor(
			&cvm.TimelockTransferRelayVirtualInstructionAccounts{
				VmAuthority:          common.GetSubsidizer().PublicKey().ToBytes(),
				VirtualTimelock:      source.State.PublicKey().ToBytes(),
				VirtualTimelockVault: source.Vault.PublicKey().ToBytes(),
				Owner:                source.VaultOwner.PublicKey().ToBytes(),
				RelayVault:           commitmentVault.PublicKey().ToBytes(),
			},
			&cvm.TimelockTransferRelayVirtualInstructionArgs{
				TimelockBump: source.StateBump,
				Amount:       kinAmountInQuarks,
				Signature:    cvm.Signature(virtualSignature),
			},
		),
	)

	execInstruction := cvm.NewVmExecInstruction(
		&cvm.VmExecInstructionAccounts{
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          &memoryAPublicKeyBytes,
			VmMemB:          &memoryBPublicKeyBytes,
			VmMemC:          &memoryCPublicKeyBytes,
			VmOmnibus:       &vmOmnibusPublicKeyBytes,
			VmRelay:         &treasuryPoolPublicKeyBytes,
			VmRelayVault:    &treasuryPoolVaultPublicKeyBytes,
			ExternalAddress: &treasuryPoolVaultPublicKeyBytes,
		},
		&cvm.VmExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, relayIndex},
			MemBanks:   []uint8{0, 1, 2},
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}
