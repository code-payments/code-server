package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
)

type store struct {
	mu      sync.Mutex
	records []*twitter.Record
	last    uint64
}

// New returns a new in memory twitter.Store
func New() twitter.Store {
	return &store{}
}

// Put implements twitter.Store.Save
func (s *store) Save(_ context.Context, data *twitter.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		data.LastUpdatedAt = time.Now()

		item.Name = data.Name
		item.ProfilePicUrl = data.ProfilePicUrl
		item.VerifiedType = data.VerifiedType
		item.FollowerCount = data.FollowerCount
		item.TipAddress = data.TipAddress
		item.LastUpdatedAt = data.LastUpdatedAt
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		data.LastUpdatedAt = time.Now()

		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

// Get implements twitter.Store.Get
func (s *store) Get(_ context.Context, username string) (*twitter.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByUsername(username)
	if item == nil {
		return nil, twitter.ErrUserNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

func (s *store) find(data *twitter.Record) *twitter.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if data.Username == item.Username {
			return item
		}
	}

	return nil
}

func (s *store) findByUsername(username string) *twitter.Record {
	for _, item := range s.records {
		if username == item.Username {
			return item
		}
	}

	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
