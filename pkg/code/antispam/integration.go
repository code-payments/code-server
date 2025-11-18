package antispam

import (
	"context"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
)

// Integration is an antispam guard integration that apps can implement to check
// whether operations of interest are allowed to be performed.
type Integration interface {
	AllowOpenAccounts(ctx context.Context, owner *common.Account, accountSet transactionpb.OpenAccountsMetadata_AccountSet) (bool, string, error)

	AllowWelcomeBonus(ctx context.Context, owner *common.Account) (bool, string, error)

	AllowSendPayment(ctx context.Context, owner, destination *common.Account, isPublic bool) (bool, string, error)

	AllowReceivePayments(ctx context.Context, owner *common.Account, isPublic bool) (bool, string, error)

	AllowDistribution(ctx context.Context, owner *common.Account, isPublic bool) (bool, string, error)

	AllowSwap(ctx context.Context, owner, fromMint, toMint *common.Account) (bool, string, error)
}

type allowEverythingIntegration struct {
}

// NewAllowEverything returns a default antispam integration that allows everything
func NewAllowEverything() Integration {
	return &allowEverythingIntegration{}
}

func (i *allowEverythingIntegration) AllowOpenAccounts(ctx context.Context, owner *common.Account, accountSet transactionpb.OpenAccountsMetadata_AccountSet) (bool, string, error) {
	return true, "", nil
}

func (i *allowEverythingIntegration) AllowWelcomeBonus(ctx context.Context, owner *common.Account) (bool, string, error) {
	return true, "", nil
}

func (i *allowEverythingIntegration) AllowSendPayment(ctx context.Context, owner, destination *common.Account, isPublic bool) (bool, string, error) {
	return true, "", nil
}

func (i *allowEverythingIntegration) AllowReceivePayments(ctx context.Context, owner *common.Account, isPublic bool) (bool, string, error) {
	return true, "", nil
}

func (i *allowEverythingIntegration) AllowDistribution(ctx context.Context, owner *common.Account, isPublic bool) (bool, string, error) {
	return true, "", nil
}

func (i *allowEverythingIntegration) AllowSwap(ctx context.Context, owner, fromMint, toMint *common.Account) (bool, string, error) {
	return true, "", nil
}
