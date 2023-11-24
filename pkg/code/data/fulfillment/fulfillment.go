package fulfillment

import (
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/phone"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

var (
	ErrFulfillmentNotFound = errors.New("no records could be found")
	ErrFulfillmentExists   = errors.New("fulfillment exists")
	ErrInvalidFulfillment  = errors.New("invalid fulfillment")
)

type Type uint8

const (
	UnknownType Type = iota
	InitializeLockedTimelockAccount
	NoPrivacyTransferWithAuthority
	NoPrivacyWithdraw
	TemporaryPrivacyTransferWithAuthority
	PermanentPrivacyTransferWithAuthority
	TransferWithCommitment
	CloseEmptyTimelockAccount
	CloseDormantTimelockAccount
	SaveRecentRoot
	InitializeCommitmentProof
	UploadCommitmentProof
	VerifyCommitmentProof // Deprecated, since we bundle verification with OpenCommitmentVault
	OpenCommitmentVault
	CloseCommitmentVault
)

type State uint8

const (
	StateUnknown   State = iota // not scheduled
	StatePending                // submitted to the solana network
	StateRevoked                // tx not submitted and revoked
	StateConfirmed              // tx confirmed
	StateFailed                 // tx failed
)

type Record struct {
	Id uint64

	Intent     string
	IntentType intent.Type

	ActionId   uint32
	ActionType action.Type

	FulfillmentType Type
	Data            []byte
	Signature       *string

	Nonce     *string
	Blockhash *string

	Source      string  // Source token account involved in the transaction
	Destination *string // Destination token account involved in the transaction, when it makes sense (eg. transfers)

	// Metadata required to pre-sort fulfillments for scheduling
	//
	// This is a 3-tiered sorting heurstic. At each tier, f1 < f2 when index1 < index2.
	// We move down in tiers when the current level tier matches. The order of tiers is
	// intent, then action, then fullfillment.
	IntentOrderingIndex      uint64 // Typically, but not always, the FK to the intent ID
	ActionOrderingIndex      uint32 // Typically, but not always, the FK of the action ID
	FulfillmentOrderingIndex uint32

	// Does the fulfillment worker poll for this record? If true, it's up to other
	// systems to hint as to when polling can occur. This is primarily an optimization
	// to reduce redundant processing. This doesn't affect correctness of scheduling
	// (eg. depedencies), so accidentally making some actively scheduled is ok.
	DisableActiveScheduling bool

	// Metadata required to help make antispam decisions
	InitiatorPhoneNumber *string

	State State

	CreatedAt time.Time
}

type BySchedulingOrder []*Record

func (a BySchedulingOrder) Len() int           { return len(a) }
func (a BySchedulingOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a BySchedulingOrder) Less(i, j int) bool { return a[i].ScheduledBefore(a[j]) }

func (r *Record) IsFulfilled() bool {
	return r.State == StateConfirmed
}

func (r *Record) ScheduledBefore(other *Record) bool {
	if other == nil {
		return true
	}

	if r.Id == other.Id {
		return false
	}

	if r.IntentOrderingIndex > other.IntentOrderingIndex {
		return false
	} else if r.IntentOrderingIndex < other.IntentOrderingIndex {
		return true
	}

	if r.ActionOrderingIndex > other.ActionOrderingIndex {
		return false
	} else if r.ActionOrderingIndex < other.ActionOrderingIndex {
		return true
	}

	return r.FulfillmentOrderingIndex < other.FulfillmentOrderingIndex
}

func (r *Record) Clone() Record {
	var data []byte
	if r.Data != nil {
		data = make([]byte, len(r.Data))
		copy(data, r.Data)
	}

	return Record{
		Id:                       r.Id,
		Intent:                   r.Intent,
		IntentType:               r.IntentType,
		ActionId:                 r.ActionId,
		ActionType:               r.ActionType,
		FulfillmentType:          r.FulfillmentType,
		Data:                     data,
		Signature:                pointer.StringCopy(r.Signature),
		Nonce:                    pointer.StringCopy(r.Nonce),
		Blockhash:                pointer.StringCopy(r.Blockhash),
		Source:                   r.Source,
		Destination:              pointer.StringCopy(r.Destination),
		IntentOrderingIndex:      r.IntentOrderingIndex,
		ActionOrderingIndex:      r.ActionOrderingIndex,
		FulfillmentOrderingIndex: r.FulfillmentOrderingIndex,
		DisableActiveScheduling:  r.DisableActiveScheduling,
		InitiatorPhoneNumber:     pointer.StringCopy(r.InitiatorPhoneNumber),
		State:                    r.State,
		CreatedAt:                r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.Intent = r.Intent
	dst.IntentType = r.IntentType
	dst.ActionId = r.ActionId
	dst.ActionType = r.ActionType
	dst.FulfillmentType = r.FulfillmentType
	dst.Data = r.Data
	dst.Signature = r.Signature
	dst.Nonce = r.Nonce
	dst.Blockhash = r.Blockhash
	dst.Source = r.Source
	dst.Destination = r.Destination
	dst.IntentOrderingIndex = r.IntentOrderingIndex
	dst.ActionOrderingIndex = r.ActionOrderingIndex
	dst.FulfillmentOrderingIndex = r.FulfillmentOrderingIndex
	dst.InitiatorPhoneNumber = r.InitiatorPhoneNumber
	dst.DisableActiveScheduling = r.DisableActiveScheduling
	dst.State = r.State
	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.Intent) == 0 {
		return errors.New("intent is required")
	}

	if r.IntentType == intent.UnknownType {
		return errors.New("intent type is required")
	}

	if r.ActionType == action.UnknownType {
		return errors.New("action type is required")
	}

	if r.FulfillmentType == UnknownType {
		return errors.New("fulfillment type is required")
	}

	if r.Signature != nil && len(*r.Signature) == 0 {
		return errors.New("signature is required when set")
	}

	if len(r.Data) != 0 && r.Signature == nil {
		return errors.New("signature must be set when data is present")
	}

	if r.Nonce != nil && len(*r.Nonce) == 0 {
		return errors.New("nonce is required when set")
	}

	if r.Blockhash != nil && len(*r.Blockhash) == 0 {
		return errors.New("blockhash is required when set")
	}

	if (r.Nonce == nil) != (r.Blockhash == nil) {
		return errors.New("nonce and blockhash must be set or not set at the same time")
	}

	if len(r.Source) == 0 {
		return errors.New("source token account is required")
	}

	if r.Destination != nil && len(*r.Destination) == 0 {
		return errors.New("destination token account is empty")
	}

	// todo: validate intent, action and fulfillment type all align

	if r.InitiatorPhoneNumber != nil && !phone.IsE164Format(*r.InitiatorPhoneNumber) {
		return errors.New("initiator phone number doesn't match E.164 format")
	}

	return nil
}

func (s State) IsTerminal() bool {
	switch s {
	case StateConfirmed:
		fallthrough
	case StateFailed:
		fallthrough
	case StateRevoked:
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

func (s Type) String() string {
	switch s {
	case UnknownType:
		return "unknown"
	case InitializeLockedTimelockAccount:
		return "initialize_locked_timelock_account"
	case NoPrivacyTransferWithAuthority:
		return "no_privacy_transfer_with_authority"
	case NoPrivacyWithdraw:
		return "no_privacy_withdraw"
	case TemporaryPrivacyTransferWithAuthority:
		return "temporary_privacy_transfer_with_authority"
	case PermanentPrivacyTransferWithAuthority:
		return "permanent_privacy_transfer_with_authority"
	case TransferWithCommitment:
		return "transfer_with_commitment"
	case CloseEmptyTimelockAccount:
		return "close_empty_timelock_account"
	case CloseDormantTimelockAccount:
		return "close_dormant_timelock_account"
	case SaveRecentRoot:
		return "save_recent_root"
	case InitializeCommitmentProof:
		return "initialize_commitment_proof"
	case UploadCommitmentProof:
		return "upload_commitment_proof"
	case VerifyCommitmentProof:
		return "verify_commitment_proof"
	case OpenCommitmentVault:
		return "open_commitment_vault"
	case CloseCommitmentVault:
		return "close_commitment_vault"
	}

	return "unknown"
}
