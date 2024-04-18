package memory

import (
	"context"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/user"
	user_identity "github.com/code-payments/code-server/pkg/code/data/user/identity"
)

type store struct {
	mu                 sync.RWMutex
	usersByID          map[string]*user_identity.Record
	usersByPhoneNumber map[string]*user_identity.Record
}

// New returns an in memory user_identity.Store
func New() user_identity.Store {
	return &store{
		usersByID:          make(map[string]*user_identity.Record),
		usersByPhoneNumber: make(map[string]*user_identity.Record),
	}
}

// Put implements user_identity.Store.Put
func (s *store) Put(ctx context.Context, record *user_identity.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.usersByID[record.ID.String()]
	if ok {
		return user_identity.ErrAlreadyExists
	}

	_, ok = s.usersByPhoneNumber[*record.View.PhoneNumber]
	if ok {
		return user_identity.ErrAlreadyExists
	}

	cpy := &user_identity.Record{
		ID: record.ID,
		View: &user.View{
			PhoneNumber: record.View.PhoneNumber,
		},
		IsStaffUser: record.IsStaffUser,
		IsBanned:    record.IsBanned,
		CreatedAt:   record.CreatedAt,
	}

	s.usersByID[record.ID.String()] = cpy
	s.usersByPhoneNumber[*record.View.PhoneNumber] = cpy

	return nil
}

// GetByID implements user_identity.Store.GetByID
func (s *store) GetByID(ctx context.Context, id *user.Id) (*user_identity.Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.usersByID[id.String()]
	if !ok {
		return nil, user_identity.ErrNotFound
	}

	return record, nil
}

// GetByView implements user_identity.Store.GetByView
func (s *store) GetByView(ctx context.Context, view *user.View) (*user_identity.Record, error) {
	if err := view.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.usersByPhoneNumber[*view.PhoneNumber]
	if !ok {
		return nil, user_identity.ErrNotFound
	}

	return record, nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.usersByID = make(map[string]*user_identity.Record)
	s.usersByPhoneNumber = make(map[string]*user_identity.Record)
}
