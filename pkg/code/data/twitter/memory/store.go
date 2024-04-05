package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/twitter"
)

type ByLastUpdatedAt []*twitter.Record

func (a ByLastUpdatedAt) Len() int           { return len(a) }
func (a ByLastUpdatedAt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByLastUpdatedAt) Less(i, j int) bool { return a[i].LastUpdatedAt.Before(a[j].LastUpdatedAt) }

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

	itemByTipAddress := s.findUserByTipAddress(data.TipAddress)
	if itemByTipAddress != nil && data.Username != itemByTipAddress.Username {
		return twitter.ErrDuplicateTipAddress
	}

	if item := s.findUser(data); item != nil {
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

// GetUserByUsername implements twitter.Store.GetUserByUsername
func (s *store) GetUserByUsername(_ context.Context, username string) (*twitter.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findUserByUsername(username)
	if item == nil {
		return nil, twitter.ErrUserNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetUserByTipAddress implements twitter.Store.GetUserByTipAddress
func (s *store) GetUserByTipAddress(ctx context.Context, tipAddress string) (*twitter.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findUserByTipAddress(tipAddress)
	if item == nil {
		return nil, twitter.ErrUserNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetStaleUsers implements twitter.Store.GetStaleUsers
func (s *store) GetStaleUsers(ctx context.Context, minAge time.Duration, limit int) ([]*twitter.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findStaleUsers(minAge)

	sorted := ByLastUpdatedAt(items)
	sort.Sort(sorted)

	if len(items) > limit {
		sorted = sorted[:limit]
	}

	if len(sorted) == 0 {
		return nil, twitter.ErrUserNotFound
	}
	return userSliceCopy(sorted), nil
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

func (s *store) findUser(data *twitter.Record) *twitter.Record {
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

func (s *store) findUserByUsername(username string) *twitter.Record {
	for _, item := range s.userRecords {
		if username == item.Username {
			return item
		}
	}

	return nil
}

func (s *store) findUserByTipAddress(tipAddress string) *twitter.Record {
	for _, item := range s.userRecords {
		if tipAddress == item.TipAddress {
			return item
		}
	}

	return nil
}

func (s *store) findStaleUsers(minAge time.Duration) []*twitter.Record {
	var res []*twitter.Record
	for _, item := range s.userRecords {
		if time.Since(item.LastUpdatedAt) > minAge {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userRecords = nil
	s.processedTweets = make(map[string]any)
	s.last = 0
}

func userSliceCopy(items []*twitter.Record) []*twitter.Record {
	res := make([]*twitter.Record, len(items))
	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}
	return res
}
