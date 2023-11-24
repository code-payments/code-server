package data

import (
	"context"
	"errors"

	"github.com/bits-and-blooms/bloom/v3"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	estimatedProviderMetricsName = "data.estimated_provider"
)

var (
	maxEstimatedAccounts        = 1000000
	maxEstimatedAccountsErrRate = 0.01
)

var (
	ErrInvalidAccount = errors.New("invalid account")
)

type EstimatedData interface {
	// Account
	// --------------------------------------------------------------------------------

	TestForKnownAccount(ctx context.Context, account []byte) (bool, error)
	AddKnownAccount(ctx context.Context, account []byte) error
}

type EstimatedProvider struct {
	knownAccounts *bloom.BloomFilter
}

func NewEstimatedProvider() (EstimatedData, error) {
	filter := bloom.NewWithEstimates(uint(maxEstimatedAccounts), maxEstimatedAccountsErrRate)

	return &EstimatedProvider{
		knownAccounts: filter,
	}, nil
}

// Account
// --------------------------------------------------------------------------------

func (p *EstimatedProvider) TestForKnownAccount(ctx context.Context, account []byte) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, estimatedProviderMetricsName, "TestForKnownAccount")
	defer tracer.End()

	if len(account) > 0 {
	} else {
		return false, ErrInvalidAccount
	}
	return p.knownAccounts.Test(account), nil
}

func (p *EstimatedProvider) AddKnownAccount(ctx context.Context, account []byte) error {
	tracer := metrics.TraceMethodCall(ctx, estimatedProviderMetricsName, "AddKnownAccount")
	defer tracer.End()

	if len(account) > 0 {
		p.knownAccounts.Add(account)
	} else {
		return ErrInvalidAccount
	}
	return nil
}
