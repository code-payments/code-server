package transaction_v2

import (
	"context"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
)

type SubmitIntentIntegration interface {
	// AllowCreation determines whether the new intent creation should be allowed
	// with app-specific validation rules
	AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) error

	// OnSuccess is a best-effort callback when an intent has been successfully
	// submitted
	OnSuccess(ctx context.Context, intentRecord *intent.Record) error
}

type defaultSubmitIntentIntegration struct {
}

// NewDefaultSubmitIntentIntegration retuns a SubmitIntentIntegration that allows everything
func NewDefaultSubmitIntentIntegration() SubmitIntentIntegration {
	return &defaultSubmitIntentIntegration{}
}

func (i *defaultSubmitIntentIntegration) AllowCreation(ctx context.Context, intentRecord *intent.Record, metadata *transactionpb.Metadata, actions []*transactionpb.Action) error {
	return nil
}

func (i *defaultSubmitIntentIntegration) OnSuccess(ctx context.Context, intentRecord *intent.Record) error {
	return nil
}

type AirdropIntegration interface {
	// GetWelcomeBonusAmount returns the amount that should be paid for the
	// welcome bonus. Return 0 amount if the airdrop should not be sent.
	GetWelcomeBonusAmount(ctx context.Context, owner *common.Account) (float64, currency_lib.Code, error)
}

type defaultAirdropIntegration struct{}

// NewDefaultAirdropIntegration retuns an AirdropIntegration that sends $1 USD
// to everyone
func NewDefaultAirdropIntegration() AirdropIntegration {
	return &defaultAirdropIntegration{}
}

func (i *defaultAirdropIntegration) GetWelcomeBonusAmount(ctx context.Context, owner *common.Account) (float64, currency_lib.Code, error) {
	return 1.0, currency_lib.USD, nil
}
