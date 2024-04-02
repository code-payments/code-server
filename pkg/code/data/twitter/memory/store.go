package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
)

type store struct {
	mu              sync.Mutex
	userRecords     []*twitter.Record
	processedTweets map[string]any
	last            uint64
}

// New returns a new in memory twitter.Store
func New() twitter.Store {
	return &store{
		processedTweets: make(map[string]any),
	}
}

// SaveUser implements twitter.Store.SaveUser
func (s *store) SaveUser(_ context.Context, data *twitter.Record) error {
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
		s.userRecords = append(s.userRecords, &c)
	}

	return nil
}

// GetUser implements twitter.Store.GetUser
func (s *store) GetUser(_ context.Context, username string) (*twitter.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByUsername(username)
	if item == nil {
		return nil, twitter.ErrUserNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// MarkTweetAsProcessed implements twitter.Store.MarkTweetAsProcessed
func (s *store) MarkTweetAsProcessed(_ context.Context, tweetId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.processedTweets[tweetId] = struct{}{}
	return nil
}

// IsTweetProcessed implements twitter.Store.IsTweetProcessed
func (s *store) IsTweetProcessed(_ context.Context, tweetId string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.processedTweets[tweetId]
	return ok, nil
}

func (s *store) find(data *twitter.Record) *twitter.Record {
	for _, item := range s.userRecords {
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
	for _, item := range s.userRecords {
		if username == item.Username {
			return item
		}
	}

	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userRecords = nil
	s.processedTweets = make(map[string]any)
	s.last = 0
}
