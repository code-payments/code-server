package action

import (
	"errors"
	"time"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/pointer"
)

type Type uint8

const (
	UnknownType Type = iota
	OpenAccount
	CloseEmptyAccount
	CloseDormantAccount // Deprecated by the VM
	NoPrivacyTransfer
	NoPrivacyWithdraw
	PrivateTransfer // Deprecated privacy flow
	SaveRecentRoot  // Deprecated privacy flow
)

type State uint8

const (
	StateUnknown State = iota
	StatePending
	StateRevoked
	StateConfirmed
	StateFailed
)

type ByActionId []*Record

func (a ByActionId) Len() int           { return len(a) }
func (a ByActionId) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByActionId) Less(i, j int) bool { return a[i].ActionId < a[j].ActionId }

type Record struct {
	Id uint64

	Intent     string
	IntentType intent.Type

	ActionId   uint32
	ActionType Type

	Source      string  // Source token account involved
	Destination *string // Destination token account involved, when it makes sense

	// Core mint quark amount involved, when it makes sense. This must be set for actions
	// that make balance changes across Code accounts! For deferred actions that are
	// initially in the unknown state, the balance may be nil and updated at a later
	// time. Store implementations will enforce which actions will allow quantity updates.
	//
	// todo: We have some options of how to handle balance calculations for actions in
	//       the unknown state. For now, remain in the most flexible state (ie. set quantity
	//       as needed and include everything in the calculation). We'll wait for more
	//       use cases before forming a firm opinion.
	Quantity *uint64

	FeeType *transactionpb.FeePaymentAction_FeeType

	State State

	Version uint64

	CreatedAt time.Time
}

func (r *Record) Validate() error {
	if len(r.Intent) == 0 {
		return errors.New("intent is required")
	}

	if r.IntentType == intent.UnknownType {
		return errors.New("intent type is required")
	}

	if r.ActionType == UnknownType {
		return errors.New("action type is required")
	}

	// todo: validate intent and action type align

	if len(r.Source) == 0 {
		return errors.New("source is required")
	}

	if r.Destination != nil && len(*r.Destination) == 0 {
		return errors.New("destination is required when set")
	}

	if r.Quantity != nil && *r.Quantity == 0 {
		return errors.New("quantity is required when set")
	}

	if r.FeeType != nil && *r.FeeType == transactionpb.FeePaymentAction_UNKNOWN {
		return errors.New("fee type is required when set")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Intent:     r.Intent,
		IntentType: r.IntentType,

		ActionId:   r.ActionId,
		ActionType: r.ActionType,

		Source:      r.Source,
		Destination: pointer.StringCopy(r.Destination),
		Quantity:    pointer.Uint64Copy(r.Quantity),

		FeeType: (*transactionpb.FeePaymentAction_FeeType)(pointer.Int32Copy((*int32)(r.FeeType))),

		State: r.State,

		Version: r.Version,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Intent = r.Intent
	dst.IntentType = r.IntentType

	dst.ActionId = r.ActionId
	dst.ActionType = r.ActionType

	dst.Source = r.Source
	dst.Destination = pointer.StringCopy(r.Destination)
	dst.Quantity = pointer.Uint64Copy(r.Quantity)

	dst.FeeType = (*transactionpb.FeePaymentAction_FeeType)(pointer.Int32Copy((*int32)(r.FeeType)))

	dst.State = r.State

	dst.Version = r.Version

	dst.CreatedAt = r.CreatedAt
}

func (s State) IsTerminal() bool {
	switch s {
	case StateConfirmed, StateFailed, StateRevoked:
		return true
	}
	return false
}

func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StatePending:
		return "pending"
	case StateConfirmed:
		return "confirmed"
	case StateFailed:
		return "failed"
	case StateRevoked:
		return "revoked"
	}

	return "unknown"
}
