package memory

import (
	"context"
	"sync"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/data/airdrop"
)

type store struct {
	mu                     sync.Mutex
	ineligibleAidropOwners map[string]any
}

// New returns a new in memory airdrop.Store
func New() airdrop.Store {
	return &store{
		ineligibleAidropOwners: make(map[string]any),
	}
}

// MarkIneligible implements airdrop.Store.MarkIneligible
func (s *store) MarkIneligible(ctx context.Context, owner string) error {
	if len(owner) == 0 {
		return errors.New("owner is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ineligibleAidropOwners[owner] = struct{}{}

	return nil
}

// IsEligible implements airdrop.Store.IsEligible
func (s *store) IsEligible(ctx context.Context, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, isIneligible := s.ineligibleAidropOwners[owner]
	return !isIneligible, nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ineligibleAidropOwners = make(map[string]any)
}
