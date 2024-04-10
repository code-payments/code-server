package transaction

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/solana/memo"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"
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

// GetTipMemoValue gets the string value provided in a memo for tips
func GetTipMemoValue(platform transactionpb.TippedUser_Platform, username string) (string, error) {
	var platformName string
	switch platform {
	case transactionpb.TippedUser_TWITTER:
		platformName = "X"
	default:
		return "", errors.New("unexpeted tip platform")
	}

	return fmt.Sprintf("tip:%s:%s", platformName, username), nil
}
