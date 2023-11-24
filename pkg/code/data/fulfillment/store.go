package fulfillment

import (
	"context"

	"github.com/code-payments/code-server/pkg/database/query"
)

type Store interface {
	// Count returns the total count of fulfillment records.
	Count(ctx context.Context) (uint64, error)

	// Count returns the total count of fulfillments in the provided state.
	CountByState(ctx context.Context, state State) (uint64, error)

	// CountByStateGroupedByType returns the total count of fulfillments, grouped
	// by type, in the provided state.
	CountByStateGroupedByType(ctx context.Context, state State) (map[Type]uint64, error)

	// CountForMetrics is like CountByStateGroupedByType for metrics. Partial data may be provided.
	CountForMetrics(ctx context.Context, state State) (map[Type]uint64, error)

	// CountByStateAndAddress returns the total count of fulfillments for the provided account and state.
	CountByStateAndAddress(ctx context.Context, state State, address string) (uint64, error)

	// CountByStateAndAddress returns the total count of fulfillments for the provided type, state and account (as a source an destination).
	CountByTypeStateAndAddress(ctx context.Context, fulfillmentType Type, state State, address string) (uint64, error)

	// CountByStateAndAddress returns the total count of fulfillments for the provided type, state and account as a source.
	CountByTypeStateAndAddressAsSource(ctx context.Context, fulfillmentType Type, state State, address string) (uint64, error)

	// Count returns the total count of fulfillments for the provided intent and state.
	CountByIntentAndState(ctx context.Context, intent string, state State) (uint64, error)

	// Count returns the total count of fulfillments for the provided intent.
	CountByIntent(ctx context.Context, intent string) (uint64, error)

	// CountByTypeActionAndState returns the total count of fulfillments with a
	// given type, action and state.
	CountByTypeActionAndState(ctx context.Context, intentId string, actionId uint32, fulfillmentType Type, state State) (uint64, error)

	// CountPendingByType gets the count of pending transactions by type.
	// This is particularly useful for estimating fees that will be consumed
	// by our subsidizer.
	CountPendingByType(ctx context.Context) (map[Type]uint64, error)

	// PutAll creates all fulfillments in one transaction
	PutAll(ctx context.Context, records ...*Record) error

	// Update updates an existing fulfillment record
	//
	// Note 1: Updating pre-sorting metadata is allowed but limited to certain fulfillment types
	// Note 2: Updating DisableActiveScheduling is done in MarkAsActivelyScheduled, due to no distributed locks existing
	Update(ctx context.Context, record *Record) error

	// GetById find the fulfillment recofd for a given ID
	GetById(ctx context.Context, id uint64) (*Record, error)

	// GetBySignature finds the fulfillment record for a given signature.
	GetBySignature(ctx context.Context, signature string) (*Record, error)

	// MarkAsActivelyScheduled marks a fulfillment as actively scheduled
	MarkAsActivelyScheduled(ctx context.Context, id uint64) error

	// ActivelyScheduleTreasuryAdvances is a specialized MarkAsActivelyScheduled variant
	// to batch enable active scheduling for treasury advances at a particular point in time
	// defined by the intent ordering index.
	ActivelyScheduleTreasuryAdvances(ctx context.Context, treasury string, intentOrderingIndex uint64, limit int) (uint64, error)

	// GetAllByState returns all fulfillment records for a given state.
	//
	// Returns ErrNotFound if no records are found.
	GetAllByState(ctx context.Context, state State, includeDisabledActiveScheduling bool, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetAllByIntent returns all fulfillment records for a given intent.
	//
	// Returns ErrNotFound if no records are found.
	GetAllByIntent(ctx context.Context, intent string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*Record, error)

	// GetAllByAction returns all fulfillment records for a given action
	//
	// Returns ErrNotFound if no records are found.
	GetAllByAction(ctx context.Context, intentId string, actionId uint32) ([]*Record, error)

	// GetAllByTypeAndAction returns all fulfillment records for a given type and action
	//
	// Returns ErrNotFound if no records are found.
	GetAllByTypeAndAction(ctx context.Context, fulfillmentType Type, intentId string, actionId uint32) ([]*Record, error)

	// GetFirstSchedulableByAddressAsSource returns the earliest fulfillment
	// that can be scheduled for an account as a source given the total ordering
	// of all fulfillments.
	//
	// Returns ErrNotFound if no records are found.
	GetFirstSchedulableByAddressAsSource(ctx context.Context, address string) (*Record, error)

	// GetFirstSchedulableByAddressAsDestination returns the earliest fulfillment
	// that can be scheduled for an account as a destination given the total ordering
	// of all fulfillments.
	//
	// Returns ErrNotFound if no records are found.
	GetFirstSchedulableByAddressAsDestination(ctx context.Context, address string) (*Record, error)

	// GetFirstSchedulableByType returns the earliest fulfillment that can be scheduled
	// for fulfillments of the provided type.
	//
	// Returns ErrNotFound if no records are found.
	GetFirstSchedulableByType(ctx context.Context, fulfillmentType Type) (*Record, error)

	// GetNextSchedulableByAddress gets the next schedulable fulfillment for an account after
	// a point in time defined by ordering indices.
	//
	// Returns ErrNotFound if no records are found.
	GetNextSchedulableByAddress(ctx context.Context, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*Record, error)
}
