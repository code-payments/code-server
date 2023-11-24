package memory

import (
	"context"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/login"
)

type store struct {
	mu      sync.Mutex
	records []*login.MultiRecord
}

// New returns a new in memory login.Store
func New() login.Store {
	return &store{}
}

// Save implements login.Store.Save
func (s *store) Save(_ context.Context, data *login.MultiRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var found bool
	for _, item := range s.records {
		if item.AppInstallId == data.AppInstallId {
			item.Owners = append([]string(nil), data.Owners...)
			item.LastUpdatedAt = time.Now()
			item.CopyTo(data)
			found = true
			continue
		}

		var owners []string
		for _, owner := range item.Owners {
			var excludeOwner bool
			for _, updatedOwner := range data.Owners {
				if owner == updatedOwner {
					excludeOwner = true
					break
				}
			}
			if !excludeOwner {
				owners = append(owners, owner)
			}
		}
		item.Owners = owners
		item.LastUpdatedAt = time.Now()
	}

	if !found {
		data.LastUpdatedAt = time.Now()
		cloned := data.Clone()
		s.records = append(s.records, &cloned)
	}

	return nil
}

// GetAllByInstallId implements login.Store.GetAllByInstallId
func (s *store) GetAllByInstallId(_ context.Context, appInstallId string) (*login.MultiRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findByAppInstallId(appInstallId); item != nil {
		if len(item.Owners) == 0 {
			return nil, login.ErrLoginNotFound
		}

		cloned := item.Clone()
		return &cloned, nil
	}

	return nil, login.ErrLoginNotFound
}

// GetLatestByOwner implements login.Store.GetLatestByOwner
func (s *store) GetLatestByOwner(_ context.Context, owner string) (*login.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByOwner(owner)
	if item == nil {
		return nil, login.ErrLoginNotFound
	}

	return &login.Record{
		AppInstallId:  item.AppInstallId,
		Owner:         owner,
		LastUpdatedAt: item.LastUpdatedAt,
	}, nil
}

func (s *store) findByAppInstallId(appInstallId string) *login.MultiRecord {
	for _, item := range s.records {
		if item.AppInstallId == appInstallId {
			return item
		}
	}
	return nil
}

func (s *store) findByOwner(owner string) *login.MultiRecord {
	for _, item := range s.records {
		for _, itemOwner := range item.Owners {
			if itemOwner == owner {
				return item
			}
		}
	}
	return nil
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
}
