package transaction

import (
	"bytes"
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

	compressInstruction := cvm.NewCompressInstruction(
		&cvm.CompressInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemory:    memory.PublicKey().ToBytes(),
			VmStorage:   storage.PublicKey().ToBytes(),
		},
		&cvm.CompressInstructionArgs{
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

	vm *common.Account,
	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,
	destinationMemory *common.Account,
	destinationIndex uint16,
) (solana.Transaction, error) {
	mergedMemoryBanks, err := MergeMemoryBanks(nonceMemory, sourceMemory, destinationMemory)
	if err != nil {
		return solana.Transaction{}, err
	}

	vixn := cvm.NewWithdrawVirtualInstruction(&cvm.WithdrawVirtualInstructionArgs{
		Signature: cvm.Signature(virtualSignature),
	})

	execInstruction := cvm.NewExecInstruction(
		&cvm.ExecInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemA:      mergedMemoryBanks.A,
			VmMemB:      mergedMemoryBanks.B,
			VmMemC:      mergedMemoryBanks.C,
		},
		&cvm.ExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   mergedMemoryBanks.Indices,
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeExternalWithdrawTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,

	vm *common.Account,
	vmOmnibus *common.Account,

	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,

	externalDestination *common.Account,
) (solana.Transaction, error) {
	mergedMemoryBanks, err := MergeMemoryBanks(nonceMemory, sourceMemory)
	if err != nil {
		return solana.Transaction{}, err
	}

	vmOmnibusPublicKeyBytes := ed25519.PublicKey(vmOmnibus.PublicKey().ToBytes())

	externalAddressPublicKeyBytes := ed25519.PublicKey(externalDestination.PublicKey().ToBytes())

	vixn := cvm.NewWithdrawVirtualInstruction(&cvm.WithdrawVirtualInstructionArgs{
		Signature: cvm.Signature(virtualSignature),
	})

	execInstruction := cvm.NewExecInstruction(
		&cvm.ExecInstructionAccounts{
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          mergedMemoryBanks.A,
			VmMemB:          mergedMemoryBanks.B,
			VmOmnibus:       &vmOmnibusPublicKeyBytes,
			ExternalAddress: &externalAddressPublicKeyBytes,
		},
		&cvm.ExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex},
			MemBanks:   mergedMemoryBanks.Indices,
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeInternalTransferWithAuthorityTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,

	vm *common.Account,
	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,
	destinationMemory *common.Account,
	destinationIndex uint16,

	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	mergedMemoryBanks, err := MergeMemoryBanks(nonceMemory, sourceMemory, destinationMemory)
	if err != nil {
		return solana.Transaction{}, err
	}

	vixn := cvm.NewTransferVirtualInstruction(&cvm.TransferVirtualInstructionArgs{
		Amount:    kinAmountInQuarks,
		Signature: cvm.Signature(virtualSignature),
	})

	execInstruction := cvm.NewExecInstruction(
		&cvm.ExecInstructionAccounts{
			VmAuthority: common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:          vm.PublicKey().ToBytes(),
			VmMemA:      mergedMemoryBanks.A,
			VmMemB:      mergedMemoryBanks.B,
			VmMemC:      mergedMemoryBanks.C,
		},
		&cvm.ExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex, destinationIndex},
			MemBanks:   mergedMemoryBanks.Indices,
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

func MakeExternalTransferWithAuthorityTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	virtualSignature solana.Signature,

	vm *common.Account,
	vmOmnibus *common.Account,

	nonceMemory *common.Account,
	nonceIndex uint16,
	sourceMemory *common.Account,
	sourceIndex uint16,

	externalDestination *common.Account,
	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	mergedMemoryBanks, err := MergeMemoryBanks(nonceMemory, sourceMemory)
	if err != nil {
		return solana.Transaction{}, err
	}

	externalAddressPublicKeyBytes := ed25519.PublicKey(externalDestination.PublicKey().ToBytes())

	vmOmnibusPublicKeyBytes := ed25519.PublicKey(vmOmnibus.PublicKey().ToBytes())

	vixn := cvm.NewExternalTransferVirtualInstruction(&cvm.TransferVirtualInstructionArgs{
		Amount:    kinAmountInQuarks,
		Signature: cvm.Signature(virtualSignature),
	})

	execInstruction := cvm.NewExecInstruction(
		&cvm.ExecInstructionAccounts{
			VmAuthority:     common.GetSubsidizer().PublicKey().ToBytes(),
			Vm:              vm.PublicKey().ToBytes(),
			VmMemA:          mergedMemoryBanks.A,
			VmMemB:          mergedMemoryBanks.B,
			VmOmnibus:       &vmOmnibusPublicKeyBytes,
			ExternalAddress: &externalAddressPublicKeyBytes,
		},
		&cvm.ExecInstructionArgs{
			Opcode:     vixn.Opcode,
			MemIndices: []uint16{nonceIndex, sourceIndex},
			MemBanks:   mergedMemoryBanks.Indices,
			Data:       vixn.Data,
		},
	)

	return MakeNoncedTransaction(nonce, bh, execInstruction)
}

type MergedMemoryBankResult struct {
	A       *ed25519.PublicKey
	B       *ed25519.PublicKey
	C       *ed25519.PublicKey
	D       *ed25519.PublicKey
	Indices []uint8
}

func MergeMemoryBanks(accounts ...*common.Account) (*MergedMemoryBankResult, error) {
	indices := make([]uint8, len(accounts))
	orderedBanks := make([]*ed25519.PublicKey, 4)

	for i, account := range accounts {
		for j, bank := range orderedBanks {
			if bank == nil {
				publicKey := ed25519.PublicKey(account.PublicKey().ToBytes())
				orderedBanks[j] = &publicKey
				indices[i] = uint8(j)
				break
			}

			if bytes.Equal(*bank, account.PublicKey().ToBytes()) {
				indices[i] = uint8(j)
				break
			}

			if j == len(orderedBanks)-1 {
				return nil, errors.New("too many memory banks")
			}
		}
	}

	return &MergedMemoryBankResult{
		A:       orderedBanks[0],
		B:       orderedBanks[1],
		C:       orderedBanks[2],
		D:       orderedBanks[3],
		Indices: indices,
	}, nil
}
