package transaction

import (
	"errors"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/code/common"
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

	timelockAccounts *common.TimelockAccounts,
) (solana.Transaction, error) {
	initializeInstruction, err := timelockAccounts.GetInitializeInstruction()
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
) (solana.Transaction, error) {
	memoInstruction, err := MakeKreMemoInstruction()
	if err != nil {
		return solana.Transaction{}, err
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

	instructions := []solana.Instruction{
		memoInstruction,
		revokeLockInstruction,
		deactivateLockInstruction,
		withdrawInstruction,
		closeInstruction,
	}
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

	treasuryPool *common.Account,
	treasuryPoolVault *common.Account,
	destination *common.Account,
	commitment *common.Account,
	treasuryPoolBump uint8,
	kinAmountInQuarks uint64,
	transcript []byte,
	recentRoot []byte,
) (solana.Transaction, error) {
	memoInstruction, err := MakeKreMemoInstruction()
	if err != nil {
		return solana.Transaction{}, err
	}

	transferWithAuthorityInstruction, err := makeTransferWithCommitmentInstruction(
		treasuryPool,
		treasuryPoolVault,
		destination,
		commitment,
		treasuryPoolBump,
		kinAmountInQuarks,
		transcript,
		recentRoot,
	)
	if err != nil {
		return solana.Transaction{}, err
	}

	instructions := []solana.Instruction{
		memoInstruction,
		transferWithAuthorityInstruction,
	}
	return MakeNoncedTransaction(nonce, bh, instructions...)
}
