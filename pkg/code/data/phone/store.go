package phone

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/phone"
)

var (
	// ErrVerificationNotFound is returned when no verification(s) are found.
	ErrVerificationNotFound = errors.New("phone verification not found")

	// ErrInvalidVerification is returned if the verification is invalid.
	ErrInvalidVerification = errors.New("verification is invalid")

	// ErrMetadataNotFound is returned when no metadata is found.
	ErrMetadataNotFound = errors.New("phone metadata not found")

	// ErrLinkingTokenNotFound is returned when no link token is found.
	ErrLinkingTokenNotFound = errors.New("linking token not found")

	// ErrEventNotFound is returned when no phone event is found.
	ErrEventNotFound = errors.New("event not found")
)

const (
	// Phone number used to migrate accounts that were used in Code prior to
	// phone verification.
	GrandFatheredPhoneNumber = "+16472222222"
)

type EventType uint8

const (
	EventTypeUnknown EventType = iota
	EventTypeVerificationCodeSent
	EventTypeCheckVerificationCode
	EventTypeVerificationCompleted
)

type Verification struct {
	PhoneNumber    string
	OwnerAccount   string
	CreatedAt      time.Time
	LastVerifiedAt time.Time
}

type LinkingToken struct {
	PhoneNumber       string
	Code              string
	CurrentCheckCount uint32
	MaxCheckCount     uint32
	ExpiresAt         time.Time
}

type Settings struct {
	PhoneNumber    string
	ByOwnerAccount map[string]*OwnerAccountSetting
}

type OwnerAccountSetting struct {
	OwnerAccount  string
	IsUnlinked    *bool
	CreatedAt     time.Time
	LastUpdatedAt time.Time
}

type Event struct {
	Type EventType

	VerificationId string

	PhoneNumber   string
	PhoneMetadata *phone.Metadata

	CreatedAt time.Time
}

type Store interface {
	// SaveVerification upserts a verification. Updates will only occur on newer
	// verifications and won't lower existing permission levels.
	SaveVerification(ctx context.Context, v *Verification) error

	// GetVerification gets a phone verification for the provided owner account
	// and phone number pair.
	GetVerification(ctx context.Context, account, phoneNumber string) (*Verification, error)

	// GetLatestVerificationForAccount gets the latest verification for a given
	// owner account.
	GetLatestVerificationForAccount(ctx context.Context, account string) (*Verification, error)

	// GetLatestVerificationForNumber gets the latest verification for a given
	// phone number.
	GetLatestVerificationForNumber(ctx context.Context, phoneNumber string) (*Verification, error)

	// GetAllVerificationsForNumber gets all phone verifications for a given
	// phoneNumber. The returned value will be order by the last verification
	// time.
	//
	// todo: May want to consider using cursors, but for now we expect the number
	//       of owner accounts per number to be relatively small.
	GetAllVerificationsForNumber(ctx context.Context, phoneNumber string) ([]*Verification, error)

	// SaveLinkingToken uperts a phone linking token.
	SaveLinkingToken(ctx context.Context, token *LinkingToken) error

	// UseLinkingToken enforces the one-time use of the token if the phone number
	// and code pair matches.
	//
	// todo: Enforce one active code per phone number with a limited numer of checks,
	//       as a security measure.
	UseLinkingToken(ctx context.Context, phoneNumber, code string) error

	// FilterVerifiedNumbers filters phone numbers that have been verified.
	FilterVerifiedNumbers(ctx context.Context, phoneNumbers []string) ([]string, error)

	// GetSettings gets settings for a phone number. The implementation guarantee
	// an empty setting is returned if the DB entry doesn't exist.
	GetSettings(ctx context.Context, phoneNumber string) (*Settings, error)

	// SaveOwnerAccountSetting saves a phone setting for a given owner account.
	// Only non-nil settings will be updated.
	SaveOwnerAccountSetting(ctx context.Context, phoneNumber string, newSettings *OwnerAccountSetting) error

	// PutEvent stores a new phone event
	PutEvent(ctx context.Context, event *Event) error

	// GetLatestEventForNumberByType gets the latest event for a phone number by type
	GetLatestEventForNumberByType(ctx context.Context, phoneNumber string, eventType EventType) (*Event, error)

	// CountEventsForVerificationByType gets the count of events by type for a given verification
	CountEventsForVerificationByType(ctx context.Context, verification string, eventType EventType) (uint64, error)

	// CountEventsForNumberByType gets the count of events by type for a given phone number since a
	// timestamp
	CountEventsForNumberByTypeSinceTimestamp(ctx context.Context, phoneNumber string, eventType EventType, since time.Time) (uint64, error)

	// CountUniqueVerificationIdsForNumberSinceTimestamp counts the number of unique verifications a
	// phone number has been involved in since a timestamp.
	CountUniqueVerificationIdsForNumberSinceTimestamp(ctx context.Context, phoneNumber string, since time.Time) (uint64, error)
}

// Validate validates a Verification
func (v *Verification) Validate() error {
	if v == nil {
		return errors.New("verification is nil")
	}

	if !phone.IsE164Format(v.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	if len(v.OwnerAccount) == 0 {
		return errors.New("owner account cannot be empty")
	}

	if v.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	if v.LastVerifiedAt.IsZero() {
		return errors.New("verification time is zero")
	}

	return nil
}

func (t *LinkingToken) Validate() error {
	if t == nil {
		return errors.New("linking token is nil")
	}

	if !phone.IsE164Format(t.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	if !phone.IsVerificationCode(t.Code) {
		return errors.New("verification code is not a 4-10 digit string")
	}

	if t.ExpiresAt.IsZero() {
		return errors.New("expiry time is zero")
	}

	if t.ExpiresAt.Before(time.Now()) {
		return errors.New("expirty time is in the past")
	}

	if t.MaxCheckCount == 0 {
		return errors.New("maximum check count must be positive")
	}

	return nil
}

func (s *Settings) Validate() error {
	if s == nil {
		return errors.New("phone settings is nil")
	}

	if !phone.IsE164Format(s.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	for ownerAccount, setting := range s.ByOwnerAccount {
		if ownerAccount != setting.OwnerAccount {
			return errors.New("invalid owner account setting map key")
		}

		if err := setting.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (s *OwnerAccountSetting) Validate() error {
	if s == nil {
		return errors.New("owner account settings is nil")
	}

	if len(s.OwnerAccount) == 0 {
		return errors.New("owner account cannot be empty")
	}

	if s.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	if s.LastUpdatedAt.IsZero() {
		return errors.New("last update time is zero")
	}

	return nil
}

func (e *Event) Validate() error {
	if e == nil {
		return errors.New("event is nil")
	}

	if e.Type == EventTypeUnknown {
		return errors.New("event type is required")
	}

	if len(e.VerificationId) == 0 {
		return errors.New("verification id is required")
	}

	if !phone.IsE164Format(e.PhoneNumber) {
		return errors.New("phone number doesn't match E.164 standard")
	}

	if e.PhoneMetadata == nil {
		return errors.New("phone metadata is required")
	}

	if e.PhoneMetadata != nil && e.PhoneNumber != e.PhoneMetadata.PhoneNumber {
		return errors.New("mismatched phone metadata detected")
	}

	if e.CreatedAt.IsZero() {
		return errors.New("creation time is zero")
	}

	return nil
}
