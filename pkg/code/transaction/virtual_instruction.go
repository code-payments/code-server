package transaction

import (
	"crypto/sha256"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

func GetVirtualTransferWithAuthorityHash(
	nonce *common.Account,
	bh solana.Blockhash,

	source *common.TimelockAccounts,
	destination *common.Account,
	kinAmountInQuarks uint64,
) (*cvm.Hash, error) {
	txn, err := GetVirtualTransferWithAuthorityTransaction(
		nonce,
		bh,
		source,
		destination,
		kinAmountInQuarks,
	)
	if err != nil {
		return nil, err
	}

	hash := getVirtualTransactionHash(&txn)
	return &hash, nil
}

func GetVirtualCloseAccountWithBalanceHash(
	nonce *common.Account,
	bh solana.Blockhash,

	source *common.TimelockAccounts,
	destination *common.Account,
) (*cvm.Hash, error) {
	txn, err := GetVirtualWithdrawTransaction(
		nonce,
		bh,
		source,
		destination,
	)
	if err != nil {
		return nil, err
	}

	hash := getVirtualTransactionHash(&txn)
	return &hash, nil
}

func GetVirtualTransferWithAuthorityTransaction(
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
	txn, err := MakeNoncedTransaction(nonce, bh, instructions...)
	if err != nil {
		return solana.Transaction{}, err
	}
	return txn, nil
}

func GetVirtualWithdrawTransaction(
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
	txn, err := MakeNoncedTransaction(nonce, bh, instructions...)
	if err != nil {
		return solana.Transaction{}, err
	}
	return txn, nil
}

func getVirtualTransactionHash(txn *solana.Transaction) cvm.Hash {
	hasher := sha256.New()
	hasher.Write(txn.Message.Marshal())
	return cvm.Hash(hasher.Sum(nil))
}
