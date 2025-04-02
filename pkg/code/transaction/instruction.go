package transaction

import (
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/system"
)

// todo: start moving instruction construction code here

func makeAdvanceNonceInstruction(nonce *common.Account) (solana.Instruction, error) {
	return system.AdvanceNonce(
		nonce.PublicKey().ToBytes(),
		common.GetSubsidizer().PublicKey().ToBytes(),
	), nil
}
