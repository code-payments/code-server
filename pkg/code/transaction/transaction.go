package transaction

import (
	"crypto/ed25519"
	"errors"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
	"github.com/code-payments/code-server/pkg/solana/memo"
)

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
	timelockAccounts *common.TimelockAccounts,
) (solana.Transaction, error) {
	burnDustInstruction, err := timelockAccounts.GetBurnDustWithAuthorityInstruction(maxBurnAmount)
	if err != nil {
		return solana.Transaction{}, err
	}

	closeInstruction, err := timelockAccounts.GetCloseAccountsInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := []solana.Instruction{
		burnDustInstruction,
		closeInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}

func MakeCloseAccountWithBalanceTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	source *common.TimelockAccounts,
	destination *common.Account,

	additionalMemo *string,
) (solana.Transaction, error) {
	originalMemoInstruction, err := MakeKreMemoInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	memoInstructions := []solana.Instruction{
		originalMemoInstruction,
	}

	if additionalMemo != nil {
		if len(*additionalMemo) == 0 {
			return solana.Transaction{}, errors.New("additional memo is empty")
		}

		additionalMemoInstruction := memo.Instruction(*additionalMemo)
		memoInstructions = append(memoInstructions, additionalMemoInstruction)
	}

	revokeLockInstruction, err := source.GetRevokeLockWithAuthorityInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	deactivateLockInstruction, err := source.GetDeactivateInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	withdrawInstruction, err := source.GetWithdrawInstruction(destination)
	if err != nil {
		return solana.Transaction{}, err
	}

	closeInstruction, err := source.GetCloseAccountsInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := append(
		memoInstructions,
		revokeLockInstruction,
		deactivateLockInstruction,
		withdrawInstruction,
		closeInstruction,
	)
	return MakeNoncedTransaction(nonce, bh, instructions...)
}

func MakeTransferWithAuthorityTransaction(
	nonce *common.Account,
	bh solana.Blockhash,

	source *common.TimelockAccounts,
	destination *common.Account,
	kinAmountInQuarks uint64,
) (solana.Transaction, error) {
	memoInstruction, err := MakeKreMemoInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	transferWithAuthorityInstruction, err := source.GetTransferWithAuthorityInstruction(destination, kinAmountInQuarks)
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := []solana.Instruction{
		memoInstruction,
		transferWithAuthorityInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}

func MakeTreasuryAdvanceTransaction(
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
	memoInstruction, err := MakeKreMemoInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

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
			Opcode:         relayTransferInternalVirtualInstruction.Opcode,
			MemIndices:     []uint16{accountIndex, relayIndex},
			MemBanks:       []uint8{0, 1},
			SignatureIndex: 0,
			Data:           relayTransferInternalVirtualInstruction.Data,
		},
	)

	instructions := []solana.Instruction{
		memoInstruction,
		execInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}
