package webhook

import (
	"time"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/pkg/errors"
)

type State uint8

const (
	StateUnknown   State = iota // Not actionable (ie. the event to trigger hasn't occurred yet)
	StatePending                // Actively sending to third party
	StateConfirmed              // Third party successfully processed webhook
	StateFailed                 // Third party failed to process webhook after sufficient retries
)

type Type uint8

const (
	TypeUnknown Type = iota
	TypeIntentSubmitted
	TypeTest
)

type Record struct {
	Id uint64

	WebhookId string
	Url       string
	Type      Type

	Attempts uint8
	State    State

	CreatedAt     time.Time
	NextAttemptAt *time.Time
}

func (r *Record) Validate() error {
	if len(r.WebhookId) == 0 {
		return errors.New("webhook id is required")
	}

	if len(r.Url) == 0 {
		return errors.New("url is required")
	}

	if r.Type == TypeUnknown {
		return errors.New("type is required")
	}

	switch r.State {
	case StatePending:
		if r.NextAttemptAt == nil || r.NextAttemptAt.IsZero() {
			return errors.New("next attempt timestamp is required")
		}
	default:
		if r.NextAttemptAt != nil {
			return errors.New("next attempt timestamp cannot be set")
		}
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		WebhookId: r.WebhookId,
		Url:       r.Url,
		Type:      r.Type,

		Attempts: r.Attempts,
		State:    r.State,

		CreatedAt:     r.CreatedAt,
		NextAttemptAt: pointer.TimeCopy(r.NextAttemptAt),
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.WebhookId = r.WebhookId
	dst.Url = r.Url
	dst.Type = r.Type

	dst.Attempts = r.Attempts
	dst.State = r.State

	dst.CreatedAt = r.CreatedAt
	dst.NextAttemptAt = pointer.TimeCopy(r.NextAttemptAt)
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
	}
	return "unknown"
}
