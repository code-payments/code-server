package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/phone"
	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
)

type store struct {
	mu                   sync.RWMutex
	usersByPhoneNumber   map[string]*invite.User
	influencerCodeByCode map[string]*invite.InfluencerCode
}

// New returns a new in memory invite.v2.Store
func New() invite.Store {
	return &store{
		usersByPhoneNumber:   make(map[string]*invite.User),
		influencerCodeByCode: make(map[string]*invite.InfluencerCode),
	}
}

// GetUser implements invite.v2.Store.GetUser
func (s *store) GetUser(_ context.Context, phoneNumber string) (*invite.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByPhoneNumber[phoneNumber]
	if !ok {
		return nil, invite.ErrUserNotFound
	}
	return user, nil
}

// PutUser implements invite.v2.Store.PutUser
func (s *store) PutUser(ctx context.Context, user *invite.User) error {
	if err := user.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copy := &invite.User{
		PhoneNumber:            user.PhoneNumber,
		InvitedBy:              user.InvitedBy,
		Invited:                user.Invited,
		InviteCount:            user.InviteCount,
		InvitesSent:            user.InvitesSent,
		DepositInvitesReceived: user.DepositInvitesReceived,
		IsRevoked:              user.IsRevoked,
	}

	_, alreadyExists := s.usersByPhoneNumber[copy.PhoneNumber]
	if alreadyExists {
		return invite.ErrAlreadyExists
	}

	if copy.InvitedBy != nil {

		if phone.IsE164Format(*copy.InvitedBy) {
			sender, ok := s.usersByPhoneNumber[*copy.InvitedBy]
			if !ok || sender.InvitesSent >= sender.InviteCount || sender.IsRevoked {
				return invite.ErrInviteCountExceeded
			}

			sender.InvitesSent++
		} else {
			influencer, ok := s.influencerCodeByCode[*copy.InvitedBy]
			if !ok || influencer.InvitesSent >= influencer.InviteCount || influencer.IsRevoked || influencer.ExpiresAt.Before(time.Now()) {
				return invite.ErrInviteCountExceeded
			}
			fmt.Printf("%+v\n", influencer)
			influencer.InvitesSent++
		}
	}

	s.usersByPhoneNumber[copy.PhoneNumber] = copy

	return nil
}

func (s *store) GetInfluencerCode(ctx context.Context, code string) (*invite.InfluencerCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var influencerCode *invite.InfluencerCode
	var ok bool

	if influencerCode, ok = s.influencerCodeByCode[code]; !ok {
		return nil, invite.ErrInfluencerCodeNotFound
	}

	return influencerCode, nil
}

func (s *store) PutInfluencerCode(ctx context.Context, influencerCode *invite.InfluencerCode) error {
	if err := influencerCode.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, alreadyExists := s.influencerCodeByCode[influencerCode.Code]
	if alreadyExists {
		return invite.ErrInfluencerCodeAlreadyExists
	}

	s.influencerCodeByCode[influencerCode.Code] = influencerCode

	return nil
}

func (s *store) ClaimInfluencerCode(ctx context.Context, code string) error {
	existingCode, err := s.GetInfluencerCode(ctx, code)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if this code has been revoked
	if existingCode.IsRevoked {
		return invite.ErrInfluencerCodeRevoked
	}

	if existingCode.InvitesSent >= existingCode.InviteCount {
		return invite.ErrInviteCountExceeded
	}

	if (!existingCode.ExpiresAt.IsZero()) && (existingCode.ExpiresAt.Before(time.Now())) {
		return invite.ErrInfluencerCodeExpired
	}

	// Update the influencer code
	s.influencerCodeByCode[code].InvitesSent++

	return nil
}

// FilterInvitedNumbers implements invite.v2.Store.FilterInvitedNumbers
func (s *store) FilterInvitedNumbers(ctx context.Context, phoneNumbers []string) ([]*invite.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []*invite.User
	for _, phoneNumber := range phoneNumbers {
		user, ok := s.usersByPhoneNumber[phoneNumber]
		if ok {
			filtered = append(filtered, user)
		}
	}

	return filtered, nil

}

// PutOnWaitlist implements invite.v2.Store.PutOnWaitlist
func (s *store) PutOnWaitlist(ctx context.Context, entry *invite.WaitlistEntry) error {
	// There's no getter methods, so we don't actually save anything
	return nil
}

// GiveInvitesForDeposit implements invite.v2.Store.GiveInvitesForDeposit
func (s *store) GiveInvitesForDeposit(ctx context.Context, phoneNumber string, amount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, ok := s.usersByPhoneNumber[phoneNumber]
	if !ok {
		return nil
	}

	if data.DepositInvitesReceived {
		return nil
	}

	data.InviteCount += uint32(amount)
	data.DepositInvitesReceived = true
	return nil
}

func (s *store) reset() {
	s.usersByPhoneNumber = make(map[string]*invite.User)
}
