package memory

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/code-payments/code-server/pkg/code/data/messaging"
)

type store struct {
	mu      sync.Mutex
	records []*messaging.Record
}

// New returns a memory backed messaging.Store.
func New() messaging.Store {
	return &store{
		records: make([]*messaging.Record, 0),
	}
}

// Insert implements messaging.Store.Insert.
func (s *store) Insert(_ context.Context, record *messaging.Record) error {
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, item := s.findByAccountAndMessageID(record.Account, record.MessageID)
	if item != nil {
		return messaging.ErrDuplicateMessageID
	}

	cloned := record.Clone()
	s.records = append(s.records, &cloned)

	return nil
}

// Delete implements messaging.Store.Delete.
func (s *store) Delete(_ context.Context, account string, messageID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i, item := s.findByAccountAndMessageID(account, messageID)
	if item == nil {
		return nil
	}

	s.records = append(s.records[:i], s.records[i+1:]...)

	return nil
}

// Get implements messaging.Store.Get.
func (s *store) Get(_ context.Context, account string) ([]*messaging.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByAccount(account)

	var copied []*messaging.Record
	for _, item := range items {
		cloned := item.Clone()
		copied = append(copied, &cloned)
	}
	return copied, nil
}

func (s *store) findByAccountAndMessageID(account string, messageID uuid.UUID) (int, *messaging.Record) {
	for i, item := range s.records {
		if item.Account != account {
			continue
		}

		if item.MessageID != messageID {
			continue
		}

		return i, item
	}

	return 0, nil
}

func (s *store) findByAccount(account string) []*messaging.Record {
	var res []*messaging.Record
	for _, item := range s.records {
		if item.Account == account {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]*messaging.Record, 0)
}
