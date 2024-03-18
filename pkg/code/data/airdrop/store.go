package airdrop

import "context"

type Store interface {
	// MarkIneligible marks an owner account as being ineligible for aidrop
	MarkIneligible(ctx context.Context, owner string) error

	// IsEligibile returns whether an owner account is eligible for airdrops
	IsEligible(ctx context.Context, owner string) (bool, error)
}
