package action

import (
	"context"
	"errors"
)

var (
	ErrActionNotFound       = errors.New("no action ecord could be found")
	ErrMultipleActionsFound = errors.New("multiple action records found")
	ErrActionExists         = errors.New("action record already exists")
)

type Store interface {
	// PutAll creates all actions in one transaction
	PutAll(ctx context.Context, records ...*Record) error

	// Update updates an existing action record
	Update(ctx context.Context, record *Record) error

	// GetById gets an action by its ID
	GetById(ctx context.Context, intent string, actionId uint32) (*Record, error)

	// GetAllByIntent gets all actions for a given intent
	GetAllByIntent(ctx context.Context, intent string) ([]*Record, error)

	// GetAllByAddress gets all actions for a given address as a source or destination.
	//
	// todo: Support paging for accounts that might have many actions when a use case emerges
	GetAllByAddress(ctx context.Context, address string) ([]*Record, error)

	// GetNetBalance gets the net balance of Kin in quarks after appying actions
	// that operate on balances.
	GetNetBalance(ctx context.Context, account string) (int64, error)

	// GetNetBalanceBatch is like GetNetBalance, but for a batch of accounts.
	GetNetBalanceBatch(ctx context.Context, accounts ...string) (map[string]int64, error)

	// GetGiftCardClaimedAction gets the action where the gift card was claimed,
	// which is a NoPrivacyWithdraw with the giftCardVault as a source. This DB
	// cannot validate the account type, so that must be done prior to making this
	// call elsewhere.
	GetGiftCardClaimedAction(ctx context.Context, giftCardVault string) (*Record, error)

	// GetGiftCardAutoReturnAction gets the action where the gift card will be
	// auto-returned, which is a CloseDormantAccount action with the giftCardVault
	// as a source. This DB cannot validate the account type, so that must be done
	// prior to making this call elsewhere.
	GetGiftCardAutoReturnAction(ctx context.Context, giftCardVault string) (*Record, error)
}
