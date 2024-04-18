package event

import (
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
)

type Type uint32

const (
	UnknownEvent Type = iota
	AccountCreated
	WelcomeBonusClaimed
	InPersonGrab
	RemoteSend
	MicroPayment
	Withdrawal
)

type Record struct {
	Id uint64

	// A common ID generally agreed upon. For example, a money movement flow using
	// multiple intents may want to standardize on which intent ID is used to add
	// additional metadata as it becomes available.
	EventId   string
	EventType Type

	// Involved accounts
	SourceCodeAccount      string
	DestinationCodeAccount *string
	ExternalTokenAccount   *string

	// Involved identities
	SourceIdentity      string
	DestinationIdentity *string

	// Involved IP addresses, and associated metadata
	//
	// todo: Requires MaxMind data set. IP metadata be missing until we do.
	// todo: May add additional "interesting" IP metadata after reviewing MaxMind data set.
	SourceClientIp           *string
	SourceClientCity         *string
	SourceClientCountry      *string
	DestinationClientIp      *string
	DestinationClientCity    *string
	DestinationClientCountry *string

	UsdValue *float64

	// Could be a good way to trigger human reviews, or automated ban processes,
	// for example. Initially, this will always be zero until we learn more about
	// good rulesets.
	SpamConfidence float64 // [0, 1]

	CreatedAt time.Time
}

// todo: Per-event type validation than just the basic stuff defined here
func (r *Record) Validate() error {
	if len(r.EventId) == 0 {
		return errors.New("event id is required")
	}

	if r.EventType == UnknownEvent || r.EventType > Withdrawal {
		return errors.New("invalid event type")
	}

	if len(r.SourceCodeAccount) == 0 {
		return errors.New("source code account is required")
	}

	if r.DestinationCodeAccount != nil && len(*r.DestinationCodeAccount) == 0 {
		return errors.New("destination code account is required when set")
	}

	if r.ExternalTokenAccount != nil && len(*r.ExternalTokenAccount) == 0 {
		return errors.New("external token account is required when set")
	}

	if len(r.SourceIdentity) == 0 {
		return errors.New("source identity is required")
	}

	if r.DestinationIdentity != nil && len(*r.DestinationIdentity) == 0 {
		return errors.New("destination identity is required when set")
	}

	if r.SourceClientIp != nil && len(*r.SourceClientIp) == 0 {
		return errors.New("source client ip is required when set")
	}

	if r.SourceClientCity != nil && len(*r.SourceClientCity) == 0 {
		return errors.New("source client city is required when set")
	}

	if r.SourceClientCountry != nil && len(*r.SourceClientCountry) == 0 {
		return errors.New("source client country is required when set")
	}

	if r.DestinationClientIp != nil && len(*r.DestinationClientIp) == 0 {
		return errors.New("destination client ip is required when set")
	}

	if r.DestinationClientCity != nil && len(*r.DestinationClientCity) == 0 {
		return errors.New("destination client city is required when set")
	}

	if r.DestinationClientCountry != nil && len(*r.DestinationClientCountry) == 0 {
		return errors.New("destination client country is required when set")
	}

	if r.UsdValue != nil && *r.UsdValue == 0 {
		return errors.New("usd value is required when set")
	}

	if r.SpamConfidence < 0 || r.SpamConfidence > 1 {
		return errors.New("spam confidence must be in the range [0, 1]")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		EventId:   r.EventId,
		EventType: r.EventType,

		SourceCodeAccount:      r.SourceCodeAccount,
		DestinationCodeAccount: pointer.StringCopy(r.DestinationCodeAccount),
		ExternalTokenAccount:   pointer.StringCopy(r.ExternalTokenAccount),

		SourceIdentity:      r.SourceIdentity,
		DestinationIdentity: pointer.StringCopy(r.DestinationIdentity),

		SourceClientIp:           pointer.StringCopy(r.SourceClientIp),
		SourceClientCity:         pointer.StringCopy(r.SourceClientCity),
		SourceClientCountry:      pointer.StringCopy(r.SourceClientCountry),
		DestinationClientIp:      pointer.StringCopy(r.DestinationClientIp),
		DestinationClientCity:    pointer.StringCopy(r.DestinationClientCity),
		DestinationClientCountry: pointer.StringCopy(r.DestinationClientCountry),

		UsdValue: pointer.Float64Copy(r.UsdValue),

		SpamConfidence: r.SpamConfidence,

		CreatedAt: r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.EventId = r.EventId
	dst.EventType = r.EventType

	dst.SourceCodeAccount = r.SourceCodeAccount
	dst.DestinationCodeAccount = pointer.StringCopy(r.DestinationCodeAccount)
	dst.ExternalTokenAccount = pointer.StringCopy(r.ExternalTokenAccount)

	dst.SourceIdentity = r.SourceIdentity
	dst.DestinationIdentity = pointer.StringCopy(r.DestinationIdentity)

	dst.SourceClientIp = pointer.StringCopy(r.SourceClientIp)
	dst.SourceClientCity = pointer.StringCopy(r.SourceClientCity)
	dst.SourceClientCountry = pointer.StringCopy(r.SourceClientCountry)
	dst.DestinationClientIp = pointer.StringCopy(r.DestinationClientIp)
	dst.DestinationClientCity = pointer.StringCopy(r.DestinationClientCity)
	dst.DestinationClientCountry = pointer.StringCopy(r.DestinationClientCountry)

	dst.UsdValue = pointer.Float64Copy(r.UsdValue)

	dst.SpamConfidence = r.SpamConfidence

	dst.CreatedAt = r.CreatedAt
}
