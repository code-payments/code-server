package transaction_v2

import (
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
)

func getBackwardsCompatMint(protoMint *commonpb.SolanaAccountId) (*common.Account, error) {
	if protoMint == nil {
		return common.CoreMintAccount, nil
	}
	return common.NewAccountFromProto(protoMint)
}
