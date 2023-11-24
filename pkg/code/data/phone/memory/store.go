package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/phone"
)

type store struct {
	mu sync.RWMutex

	verificationsByAccount map[string][]*phone.Verification
	verificationsByNumber  map[string][]*phone.Verification

	linkingTokensByNumber map[string]*phone.LinkingToken

	settingsByNumber map[string]*phone.Settings

	eventsByNumber       map[string][]*phone.Event
	eventsByVerification map[string][]*phone.Event
}

// New returns an in memory phone.Store.
func New() phone.Store {
	return &store{
		verificationsByAccount: make(map[string][]*phone.Verification),
		verificationsByNumber:  make(map[string][]*phone.Verification),

		linkingTokensByNumber: make(map[string]*phone.LinkingToken),

		settingsByNumber: make(map[string]*phone.Settings),

		eventsByNumber:       make(map[string][]*phone.Event),
		eventsByVerification: make(map[string][]*phone.Event),
	}
}

// SaveVerification implements phone.Store.SaveVerification
func (s *store) SaveVerification(ctx context.Context, newVerification *phone.Verification) error {
	if err := newVerification.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var alreadyExists bool
	currentByAccount := s.verificationsByAccount[newVerification.OwnerAccount]
	for _, verification := range currentByAccount {
		if verification.OwnerAccount != newVerification.OwnerAccount || verification.PhoneNumber != newVerification.PhoneNumber {
			continue
		}

		if !newVerification.LastVerifiedAt.After(verification.LastVerifiedAt) {
			return phone.ErrInvalidVerification
		}

		verification.LastVerifiedAt = newVerification.LastVerifiedAt

		alreadyExists = true
		break
	}
	if !alreadyExists {
		copy := &phone.Verification{
			OwnerAccount:   newVerification.OwnerAccount,
			PhoneNumber:    newVerification.PhoneNumber,
			CreatedAt:      newVerification.CreatedAt,
			LastVerifiedAt: newVerification.LastVerifiedAt,
		}
		s.verificationsByAccount[copy.OwnerAccount] = append(s.verificationsByAccount[copy.OwnerAccount], copy)
		s.verificationsByNumber[copy.PhoneNumber] = append(s.verificationsByNumber[copy.PhoneNumber], copy)
	}

	currentByAccount = s.verificationsByAccount[newVerification.OwnerAccount]
	sort.Slice(currentByAccount, func(i, j int) bool {
		return currentByAccount[i].LastVerifiedAt.After(currentByAccount[j].LastVerifiedAt)
	})

	currentByNumber := s.verificationsByNumber[newVerification.PhoneNumber]
	sort.Slice(currentByNumber, func(i, j int) bool {
		return currentByNumber[i].LastVerifiedAt.After(currentByNumber[j].LastVerifiedAt)
	})

	return nil
}

// GetVerification implements phone.Store.GetVerification
func (s *store) GetVerification(ctx context.Context, account, phoneNumber string) (*phone.Verification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verifications, ok := s.verificationsByAccount[account]
	if !ok {
		return nil, phone.ErrVerificationNotFound
	}

	for _, verification := range verifications {
		if verification.PhoneNumber == phoneNumber {
			return verification, nil
		}
	}

	return nil, phone.ErrVerificationNotFound
}

// GetLatestVerificationForAccount implements phone.Store.GetLatestVerificationForAccount
func (s *store) GetLatestVerificationForAccount(ctx context.Context, account string) (*phone.Verification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verifications, ok := s.verificationsByAccount[account]
	if !ok {
		return nil, phone.ErrVerificationNotFound
	}

	return verifications[0], nil
}

// GetLatestVerificationForNumber implements phone.Store.GetLatestVerificationForNumber
func (s *store) GetLatestVerificationForNumber(ctx context.Context, phoneNumber string) (*phone.Verification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verifications, ok := s.verificationsByNumber[phoneNumber]
	if !ok {
		return nil, phone.ErrVerificationNotFound
	}

	return verifications[0], nil
}

// GetAllVerificationsForNumber implements phone.Store.GetAllVerificationsForNumber
func (s *store) GetAllVerificationsForNumber(ctx context.Context, phoneNumber string) ([]*phone.Verification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verifications, ok := s.verificationsByNumber[phoneNumber]
	if !ok {
		return nil, phone.ErrVerificationNotFound
	}

	return verifications, nil
}

// SaveLinkingToken implements phone.Store.SaveLinkingToken
func (s *store) SaveLinkingToken(ctx context.Context, token *phone.LinkingToken) error {
	if err := token.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copy := &phone.LinkingToken{
		PhoneNumber:       token.PhoneNumber,
		Code:              token.Code,
		ExpiresAt:         token.ExpiresAt,
		CurrentCheckCount: token.CurrentCheckCount,
		MaxCheckCount:     token.MaxCheckCount,
	}
	s.linkingTokensByNumber[copy.PhoneNumber] = copy

	return nil
}

// UseLinkingToken implements phone.Store.UseLinkingToken
func (s *store) UseLinkingToken(ctx context.Context, phoneNumber, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.linkingTokensByNumber[phoneNumber]
	if !ok {
		return phone.ErrLinkingTokenNotFound
	}

	token.CurrentCheckCount++

	if token.Code != code {
		return phone.ErrLinkingTokenNotFound
	}

	delete(s.linkingTokensByNumber, phoneNumber)

	if !ok || token.ExpiresAt.Before(time.Now()) || token.CurrentCheckCount > token.MaxCheckCount {
		return phone.ErrLinkingTokenNotFound
	}

	return nil
}

// FilterVerifiedNumbers implements phone.Store.FilterVerifiedNumbers
func (s *store) FilterVerifiedNumbers(ctx context.Context, phoneNumbers []string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []string
	for _, phoneNumber := range phoneNumbers {
		_, ok := s.verificationsByNumber[phoneNumber]
		if ok {
			filtered = append(filtered, phoneNumber)
		}
	}

	return filtered, nil
}

// GetSettings implements phone.Store.GetSettings
func (s *store) GetSettings(ctx context.Context, phoneNumber string) (*phone.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	settings, ok := s.settingsByNumber[phoneNumber]
	if !ok {
		return &phone.Settings{
			PhoneNumber: phoneNumber,
		}, nil
	}

	return settings, nil
}

// SaveOwnerAccountSetting implements phone.Store.SaveOwnerAccountSetting
func (s *store) SaveOwnerAccountSetting(ctx context.Context, phoneNumber string, newSettings *phone.OwnerAccountSetting) error {
	if err := newSettings.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copy := &phone.Settings{
		PhoneNumber:    phoneNumber,
		ByOwnerAccount: make(map[string]*phone.OwnerAccountSetting),
	}

	phoneSettings, ok := s.settingsByNumber[phoneNumber]
	if ok {
		copy = phoneSettings
	}

	currentSettings, ok := copy.ByOwnerAccount[newSettings.OwnerAccount]
	if ok {
		if newSettings.IsUnlinked != nil {
			flagCopy := *newSettings.IsUnlinked
			currentSettings.IsUnlinked = &flagCopy
		}
		currentSettings.LastUpdatedAt = newSettings.LastUpdatedAt
		return nil
	}

	copy.ByOwnerAccount[newSettings.OwnerAccount] = &phone.OwnerAccountSetting{
		OwnerAccount:  newSettings.OwnerAccount,
		IsUnlinked:    newSettings.IsUnlinked,
		CreatedAt:     newSettings.CreatedAt,
		LastUpdatedAt: newSettings.LastUpdatedAt,
	}
	s.settingsByNumber[phoneNumber] = copy

	return nil
}

// PutEvent implements phone.Store.PutEvent
func (s *store) PutEvent(ctx context.Context, event *phone.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	eventsByNumber := s.eventsByNumber[event.PhoneNumber]
	eventsByVerification := s.eventsByVerification[event.VerificationId]

	copy := &phone.Event{
		Type:           event.Type,
		VerificationId: event.VerificationId,
		PhoneNumber:    event.PhoneNumber,
		PhoneMetadata:  event.PhoneMetadata,
		CreatedAt:      event.CreatedAt,
	}

	eventsByNumber = append(eventsByNumber, copy)
	s.eventsByNumber[event.PhoneNumber] = eventsByNumber

	eventsByVerification = append(eventsByVerification, copy)
	s.eventsByVerification[event.VerificationId] = eventsByVerification

	return nil
}

// GetLatestEventForNumberByType implements phone.Store.GetLatestEventForNumberByType
func (s *store) GetLatestEventForNumberByType(ctx context.Context, phoneNumber string, eventType phone.EventType) (*phone.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latestTimestamp time.Time
	var res *phone.Event

	for _, event := range s.eventsByNumber[phoneNumber] {
		if event.Type != eventType {
			continue
		}

		if event.CreatedAt.Before(latestTimestamp) {
			continue
		}

		res = event
		latestTimestamp = event.CreatedAt
	}

	if res == nil {
		return nil, phone.ErrEventNotFound
	}
	return res, nil
}

// CountEventsForVerificationByType implements phone.Store.CountEventsForVerificationByType
func (s *store) CountEventsForVerificationByType(ctx context.Context, verification string, eventType phone.EventType) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count uint64

	for _, event := range s.eventsByVerification[verification] {
		if event.Type == eventType {
			count += 1
		}
	}

	return count, nil
}

// CountEventsForNumberByTypeSinceTimestamp implements phone.Store.CountEventsForNumberByTypeSinceTimestamp
func (s *store) CountEventsForNumberByTypeSinceTimestamp(ctx context.Context, phoneNumber string, eventType phone.EventType, since time.Time) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count uint64

	for _, event := range s.eventsByNumber[phoneNumber] {
		if event.CreatedAt.Before(since) {
			continue
		}

		if event.Type != eventType {
			continue
		}

		count += 1
	}

	return count, nil
}

// CountUniqueVerificationIdsForNumberSinceTimestamp implements phone.Store.CountUniqueVerificationIdsForNumberSinceTimestamp
func (s *store) CountUniqueVerificationIdsForNumberSinceTimestamp(ctx context.Context, phoneNumber string, since time.Time) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]struct{})

	for _, event := range s.eventsByNumber[phoneNumber] {
		if event.CreatedAt.Before(since) {
			continue
		}

		res[event.VerificationId] = struct{}{}
	}

	return uint64(len(res)), nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.verificationsByAccount = make(map[string][]*phone.Verification)
	s.verificationsByNumber = make(map[string][]*phone.Verification)

	s.linkingTokensByNumber = make(map[string]*phone.LinkingToken)

	s.settingsByNumber = make(map[string]*phone.Settings)

	s.eventsByNumber = make(map[string][]*phone.Event)
	s.eventsByVerification = make(map[string][]*phone.Event)
}
