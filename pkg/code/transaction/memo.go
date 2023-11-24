package transaction

import (
	"encoding/base64"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"
)

const (
	KreAppIndex = 268
)

// MakeKreMemoInstruction makes the KRE memo instruction required for KRE payouts
//
// todo: Deprecate this with KRE gone
func MakeKreMemoInstruction() (solana.Instruction, error) {
	kreMemo, err := kin.NewMemo(kin.HighestVersion, kin.TransactionTypeP2P, KreAppIndex, nil)
	if err != nil {
		return solana.Instruction{}, err
	}
	return memo.Instruction(base64.StdEncoding.EncodeToString(kreMemo[:])), nil
}
