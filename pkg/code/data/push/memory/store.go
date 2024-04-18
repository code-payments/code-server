package memory

import (
	"context"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type store struct {
	mu      sync.Mutex
	records []*push.Record
	last    uint64
}

// New returns a new in memory push.Store
func New() push.Store {
	return &store{}
}

// Put implements push.Store.Put
func (s *store) Put(_ context.Context, record *push.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	if item := s.find(record); item != nil {
		return push.ErrTokenExists
	}

	record.Id = s.last
	s.records = append(s.records, record.Clone())

	return nil
}

// MarkAsInvalid implements push.Store.MarkAsInvalid
func (s *store) MarkAsInvalid(ctx context.Context, pushToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByPushToken(pushToken)
	for _, item := range items {
		item.IsValid = false
	}

	return nil
}

// Delete implements push.Store.Delete
func (s *store) Delete(ctx context.Context, pushToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updated []*push.Record
	for _, item := range s.records {
		if item.PushToken != pushToken {
			updated = append(updated, item)
		}
	}
	s.records = updated

	return nil
}

// GetAllValidByDataContainer implements push.Store.GetAllValidByDataContainer
func (s *store) GetAllValidByDataContainer(_ context.Context, id *user.DataContainerID) ([]*push.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByDataContainer(id)
	res = s.filterInvalid(res)

	if len(res) == 0 {
		return nil, push.ErrTokenNotFound
	}
	return res, nil
}

func (s *store) find(data *push.Record) *push.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.DataContainerId.String() == data.DataContainerId.String() && item.PushToken == data.PushToken {
			return item
		}
	}
	return nil
}

func (s *store) findByPushToken(pushToken string) []*push.Record {
	var res []*push.Record
	for _, item := range s.records {
		if item.PushToken != pushToken {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) findByDataContainer(id *user.DataContainerID) []*push.Record {
	var res []*push.Record
	for _, item := range s.records {
		if item.DataContainerId.String() != id.String() {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) filterInvalid(items []*push.Record) []*push.Record {
	var res []*push.Record
	for _, item := range items {
		if item.IsValid {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.last = 0
}
