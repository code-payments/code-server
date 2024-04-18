package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/code-payments/code-server/pkg/code/data/payment"
	"github.com/code-payments/code-server/pkg/code/data/transaction"
	"github.com/code-payments/code-server/pkg/database/query"
)

type store struct {
	paymentRecordMu sync.Mutex
	paymentRecords  map[string]*payment.Record
	lastIndex       uint64
}

type ById []*payment.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

func New() payment.Store {
	return &store{
		paymentRecords: make(map[string]*payment.Record),
		lastIndex:      1,
	}
}

func (s *store) reset() {
	s.paymentRecordMu.Lock()
	s.paymentRecords = make(map[string]*payment.Record)
	s.lastIndex = 1
	s.paymentRecordMu.Unlock()
}

func (s *store) getPk(signature string, index uint32) string {
	return fmt.Sprint(signature, index)
}

func (s *store) Put(ctx context.Context, data *payment.Record) error {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	pk := s.getPk(data.TransactionId, data.TransactionIndex)

	if _, ok := s.paymentRecords[pk]; ok {
		return payment.ErrExists
	}

	data.Id = s.lastIndex
	data.ExchangeCurrency = strings.ToLower(data.ExchangeCurrency)
	s.paymentRecords[pk] = data
	s.lastIndex++
	return nil
}

func (s *store) Get(ctx context.Context, txId string, index uint32) (*payment.Record, error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	for _, item := range s.paymentRecords {
		if item.TransactionId == txId && item.TransactionIndex == index {
			return item, nil
		}
	}

	return nil, payment.ErrNotFound
}

func (s *store) Update(ctx context.Context, data *payment.Record) error {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	var result *payment.Record
	for _, item := range s.paymentRecords {
		if item.Id == data.Id {
			result = item
		}
	}

	if result != nil {
		result.ExchangeCurrency = strings.ToLower(data.ExchangeCurrency)
		result.Region = data.Region
		result.ExchangeRate = data.ExchangeRate
		result.UsdMarketValue = data.UsdMarketValue
		result.ConfirmationState = data.ConfirmationState
	} else {
		return payment.ErrNotFound
	}

	return nil
}

func (s *store) GetAllForTransaction(ctx context.Context, txId string) ([]*payment.Record, error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	all := make([]*payment.Record, 0)
	for _, item := range s.paymentRecords {
		if item.TransactionId == txId {
			all = append(all, item)
		}
	}

	if len(all) == 0 {
		return nil, payment.ErrNotFound
	}

	return all, nil
}

func (s *store) GetAllForAccount(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering) (result []*payment.Record, err error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	if limit == 0 {
		return nil, payment.ErrNotFound
	}

	// not ideal, but this is for testing purposes and s.paymentRecord should be small
	all := make([]*payment.Record, 0)
	for _, record := range s.paymentRecords {
		if record.Source == account || record.Destination == account {
			if ordering == query.Ascending {
				if record.Id > cursor {
					all = append(all, record)
				}
			} else {
				if cursor == 0 || record.Id < cursor {
					all = append(all, record)
				}
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
		return nil, payment.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetAllForAccountByType(ctx context.Context, account string, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) (result []*payment.Record, err error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	if limit == 0 {
		return nil, payment.ErrNotFound
	}

	// not ideal, but this is for testing purposes and s.paymentRecord should be small
	all := make([]*payment.Record, 0)
	for _, record := range s.paymentRecords {
		isSender := paymentType == payment.TypeSend && record.Source == account
		isReceiver := paymentType == payment.TypeReceive && record.Destination == account
		if isSender || isReceiver {
			if ordering == query.Ascending {
				if record.Id > cursor {
					all = append(all, record)
				}
			} else {
				if record.Id < cursor {
					all = append(all, record)
				}
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
		return nil, payment.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetAllForAccountByTypeAfterBlock(ctx context.Context, account string, block uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) (result []*payment.Record, err error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	if limit == 0 {
		return nil, payment.ErrNotFound
	}

	// not ideal, but this is for testing purposes and s.paymentRecord should be small
	all := make([]*payment.Record, 0)
	for _, record := range s.paymentRecords {
		if record.BlockId <= block {
			continue
		}

		isSender := paymentType == payment.TypeSend && record.Source == account
		isReceiver := paymentType == payment.TypeReceive && record.Destination == account
		if isSender || isReceiver {
			if ordering == query.Ascending {
				if record.Id > cursor {
					all = append(all, record)
				}
			} else {
				if record.Id < cursor {
					all = append(all, record)
				}
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
		return nil, payment.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetAllForAccountByTypeWithinBlockRange(ctx context.Context, account string, lowerBound, upperBound uint64, cursor uint64, limit uint, ordering query.Ordering, paymentType payment.Type) ([]*payment.Record, error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	if limit == 0 {
		return nil, payment.ErrNotFound
	}

	// not ideal, but this is for testing purposes and s.paymentRecord should be small
	all := make([]*payment.Record, 0)
	for _, record := range s.paymentRecords {
		if record.BlockId <= lowerBound || record.BlockId >= upperBound {
			continue
		}

		isSender := paymentType == payment.TypeSend && record.Source == account
		isReceiver := paymentType == payment.TypeReceive && record.Destination == account
		if isSender || isReceiver {
			if ordering == query.Ascending {
				if record.Id > cursor {
					all = append(all, record)
				}
			} else {
				if record.Id < cursor {
					all = append(all, record)
				}
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
		return nil, payment.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetAllExternalDepositsAfterBlock(ctx context.Context, account string, block uint64, cursor uint64, limit uint, ordering query.Ordering) ([]*payment.Record, error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	if limit == 0 {
		return nil, payment.ErrNotFound
	}

	// not ideal, but this is for testing purposes and s.paymentRecord should be small
	all := make([]*payment.Record, 0)
	for _, record := range s.paymentRecords {
		if record.IsExternal && record.Destination == account && record.BlockId > block {
			if ordering == query.Ascending {
				if record.Id > cursor {
					all = append(all, record)
				}
			} else {
				if record.Id < cursor {
					all = append(all, record)
				}
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
		return nil, payment.ErrNotFound
	}

	if len(all) < int(limit) {
		return all, nil
	}

	return all[:limit], nil
}

func (s *store) GetExternalDepositAmount(ctx context.Context, account string) (uint64, error) {
	s.paymentRecordMu.Lock()
	defer s.paymentRecordMu.Unlock()

	var res uint64
	for _, record := range s.paymentRecords {
		if record.IsExternal && record.Destination == account && record.ConfirmationState == transaction.ConfirmationFinalized {
			res += record.Quantity
		}
	}
	return res, nil
}
