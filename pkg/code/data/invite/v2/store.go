package invite

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/phone"
)

var (
	// ErrUserNotFound is returned when a user is not found, which is equivalent
	// to saying the user has not been invited
	ErrUserNotFound = errors.New("user not invited")

	// ErrInviteCountExceeded is returned when a new user is attempted to be
	// added to the database via an invite from someone who has reached
	// their max count or has never been invited.
	ErrInviteCountExceeded = errors.New("invite count exceeded")

	// ErrAlreadyExists is returned when a user already exists in the database.
	ErrAlreadyExists = errors.New("user already invited")

	// ErrInfluencerCodeAlreadyExists is returned when an influencer code already exists.
	ErrInfluencerCodeAlreadyExists = errors.New("influencer code already exists")

	// ErrInfluencerCodeNotFound is returned when an influencer code is not found.
	ErrInfluencerCodeNotFound = errors.New("influencer code not found")

	// ErrInfluencerCodeRevoked is returned when an influencer code is revoked.
	ErrInfluencerCodeRevoked = errors.New("influencer code revoked")

	// ErrInfluencerCodeExpired is returned when an influencer code is expired.
	ErrInfluencerCodeExpired = errors.New("influencer code expired")
)

// This view of a user is intentionally separated from the pkg/code/data/user
// model. Invites are going to be a temporary phase of the app, and this makes
// a clear separation of what can be removed. As it's defined right now, invites
// are also grouped by phone number, whereas a user is a 1:1 mapping between an
// ID and (phone number, token account) pair.
type User struct {
	PhoneNumber string
	InvitedBy   *string
	Invited     time.Time

	InviteCount uint32
	InvitesSent uint32

	DepositInvitesReceived bool

	IsRevoked bool
}

type InfluencerCode struct {
	Code string

	InviteCount uint32
	InvitesSent uint32

	IsRevoked bool
	ExpiresAt time.Time
}

type WaitlistEntry struct {
	PhoneNumber string
	IsVerified  bool
	CreatedAt   time.Time
}

type Store interface {
	// GetUser gets an invited user.
	GetUser(ctx context.Context, phoneNumber string) (*User, error)

	// PutUser stores an invited user. If the inviter is provided, the function
	// will verify that it is allowed to invite the user and will decrement its
	// invite count on success.
	PutUser(ctx context.Context, user *User) error

	// GetInfluencerCode gets an influencer code.
	GetInfluencerCode(ctx context.Context, code string) (*InfluencerCode, error)

	// PutInfluencerCode stores an influencer code.
	PutInfluencerCode(ctx context.Context, influencerCode *InfluencerCode) error

	// ClaimInfluencerCode claims an influencer code.
	ClaimInfluencerCode(ctx context.Context, code string) error

	// FilterInvitedNumbers filters numbers that have been invited from the
	// provided list and returns the associated set of users.
	FilterInvitedNumbers(ctx context.Context, phoneNumbers []string) ([]*User, error)

	// PutOnWaitlist puts a user on a waitlist for invitation
	PutOnWaitlist(ctx context.Context, entry *WaitlistEntry) error

	// GiveInvitesForDeposit one-time gives invites for depositing Kin into Code
	GiveInvitesForDeposit(ctx context.Context, phoneNumber string, amount int) error
}

// Validate validates an invited user model.
func (u *User) Validate() error {
	if u == nil {
		return errors.New("invite user is nil")
	}

	if !phone.IsE164Format(u.PhoneNumber) {
		return errors.New("invitee phone number doesn't match E.164 standard")
	}

	if u.Invited.IsZero() {
		return errors.New("invitation time is zero")
	}

	if u.InviteCount < u.InvitesSent {
		return errors.New("invites sent cannot exceed invite count")
	}

	return nil
}

func (i *InfluencerCode) Validate() error {
	if i == nil {
		return errors.New("influencer code is nil")
	}

	if i.Code == "" {
		return errors.New("influencer code is empty")
	}

	return nil
}
