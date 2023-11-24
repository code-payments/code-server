package transaction

import (
	"github.com/code-payments/code-server/pkg/solana"
	splitter_token "github.com/code-payments/code-server/pkg/solana/splitter"
	"github.com/code-payments/code-server/pkg/solana/system"
	"github.com/code-payments/code-server/pkg/code/common"
)

// todo: start moving instruction construction code here

func makeAdvanceNonceInstruction(nonce *common.Account) (solana.Instruction, error) {
	return system.AdvanceNonce(
		nonce.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
	), nil
}

func makeTransferWithCommitmentInstruction(
	treasuryPool *common.Account,
	treasuryPoolVault *common.Account,
	destination *common.Account,
	commitment *common.Account,
	treasuryPoolBump uint8,
	kinAmountInQuarks uint64,
	transcript []byte,
	recentRoot []byte,
) (solana.Instruction, error) {
	return splitter_token.NewTransferWithCommitmentInstruction(
		&splitter_token.TransferWithCommitmentInstructionAccounts{
			Pool:        treasuryPool.PublicKey().ToBytes(),
			Vault:       treasuryPoolVault.PublicKey().ToBytes(),
			Destination: destination.PublicKey().ToBytes(),
			Commitment:  commitment.PublicKey().ToBytes(),
			Authority:   common.GetSubsidizer().PublicKey().ToBytes(),
			Payer:       common.GetSubsidizer().PublicKey().ToBytes(),
		},
		&splitter_token.TransferWithCommitmentInstructionArgs{
			PoolBump:   treasuryPoolBump,
			Amount:     kinAmountInQuarks,
			Transcript: transcript,
			RecentRoot: recentRoot,
		},
	).ToLegacyInstruction(), nil
}
