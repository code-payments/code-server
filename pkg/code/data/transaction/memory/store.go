package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
)

type store struct {
	transactionStoreMu sync.Mutex
	transactionStore   []*transaction.Record
	lastIndex          uint64
}

type ById []*transaction.Record

func (a ById) Len() int      { return len(a) }
func (a ById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

func New() transaction.Store {
	return &store{
		transactionStore: make([]*transaction.Record, 0),
		lastIndex:        0,
	}
}

func (s *store) reset() {
	s.transactionStoreMu.Lock()
	s.transactionStore = make([]*transaction.Record, 0)
	s.lastIndex = 0
	s.transactionStoreMu.Unlock()
}

// Put saves transaction data to the store.
//
// ErrExists is returned if a transaction with the same signature already exists.
func (s *store) Put(ctx context.Context, data *transaction.Record) error {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()
	s.lastIndex++

	var clone transaction.Record
	clone = *data

	for index, item := range s.transactionStore {
		if item.Signature == data.Signature {
			clone.Id = item.Id
			s.transactionStore[index] = &clone
			return nil
		}
	}

	clone.Id = s.lastIndex
	s.transactionStore = append(s.transactionStore, &clone)

	return nil
}

// Get returns a transaction record for the given signature.
//
// ErrNotFound is returned if no record is found.
func (s *store) Get(ctx context.Context, sig string) (*transaction.Record, error) {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()

	for _, item := range s.transactionStore {
		if item.Signature == sig {
			return item, nil
		}
	}

	return nil, transaction.ErrNotFound
}

func (s *store) GetAllByAddress(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering) (result []*transaction.Record, err error) {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()

	if limit == 0 {
		return nil, transaction.ErrNotFound
	}

	// not ideal, but this is for testing purposes
	all := make([]*transaction.Record, 0)
	for _, record := range s.transactionStore {
		for _, tb := range record.TokenBalances {
			if tb.Account == account {
				if cursor > 0 {
					if ordering == query.Ascending {
						if record.Id > cursor {
							all = append(all, record)
						}
					} else {
						if record.Id < cursor {
							all = append(all, record)
						}
					}
				} else {
					all = append(all, record)
				}
				break
			}
		}
	}

	sort.Sort(ById(all))

	if ordering == query.Descending {
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
	}

	if len(all) == 0 {
		return nil, transaction.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetLatestByState(ctx context.Context, account string, state transaction.Confirmation) (*transaction.Record, error) {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()

	// not ideal, but this is for testing purposes
	all := make([]*transaction.Record, 0)
	for _, record := range s.transactionStore {
		for _, tb := range record.TokenBalances {
			if tb.Account == account && record.ConfirmationState == state {
				all = append(all, record)
				break
			}
		}
	}

	sort.Sort(ById(all))

	if len(all) == 0 {
		return nil, transaction.ErrNotFound
	}

	return all[len(all)-1], nil
}

func (s *store) GetFirstPending(ctx context.Context, account string) (*transaction.Record, error) {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()

	// not ideal, but this is for testing purposes
	all := make([]*transaction.Record, 0)
	for _, record := range s.transactionStore {
		for _, tb := range record.TokenBalances {
			if tb.Account == account && record.ConfirmationState < transaction.ConfirmationFinalized {
				all = append(all, record)
				break
			}
		}
	}

	sort.Sort(ById(all))

	if len(all) == 0 {
		return nil, transaction.ErrNotFound
	}

	return all[0], nil
}

func (s *store) GetSignaturesByState(ctx context.Context, filter transaction.Confirmation, limit uint, ordering query.Ordering) ([]string, error) {
	s.transactionStoreMu.Lock()
	defer s.transactionStoreMu.Unlock()

	sort.Sort(ById(s.transactionStore))

	var results []string
	for _, item := range s.transactionStore {
		if len(results) < int(limit) && item.ConfirmationState == filter {
			results = append(results, item.Signature)
		}
	}

	if len(results) == 0 {
		return nil, transaction.ErrNotFound
	}

	return results, nil
}
