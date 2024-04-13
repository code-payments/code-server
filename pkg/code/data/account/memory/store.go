package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/pointer"
)

type store struct {
	mu      sync.Mutex
	records []*account.Record
	last    uint64
}

type ById []*account.Record

func (a ById) Len() int      { return len(a) }
func (a ById) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

type ByIndex []*account.Record

func (a ByIndex) Len() int      { return len(a) }
func (a ByIndex) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByIndex) Less(i, j int) bool {
	return a[i].Index < a[j].Index
}

type ByDepositsLastSyncedAt []*account.Record

func (a ByDepositsLastSyncedAt) Len() int      { return len(a) }
func (a ByDepositsLastSyncedAt) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByDepositsLastSyncedAt) Less(i, j int) bool {
	return a[i].DepositsLastSyncedAt.Unix() < a[j].DepositsLastSyncedAt.Unix()
}

type ByCreatedAt []*account.Record

func (a ByCreatedAt) Len() int      { return len(a) }
func (a ByCreatedAt) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByCreatedAt) Less(i, j int) bool {
	return a[i].CreatedAt.Unix() < a[j].CreatedAt.Unix()
}

func New() account.Store {
	return &store{
		records: make([]*account.Record, 0),
		last:    1,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = make([]*account.Record, 0)
}

func (s *store) find(data *account.Record) *account.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}

		if item.TokenAccount == data.TokenAccount {
			return item
		}
	}

	return nil
}

func (s *store) findByOwnerAddress(address string) []*account.Record {
	var res []*account.Record

	for _, item := range s.records {
		if item.OwnerAccount == address {
			res = append(res, item)
		}
	}

	return res
}

func (s *store) findByAuthorityAddress(address string) *account.Record {
	for _, item := range s.records {
		if item.AuthorityAccount == address {
			return item
		}
	}

	return nil
}

func (s *store) findByTokenAddress(address string) *account.Record {
	for _, item := range s.records {
		if item.TokenAccount == address {
			return item
		}
	}

	return nil
}

func (s *store) findByRequiringDepositSync(want bool) []*account.Record {
	var res []*account.Record
	for _, item := range s.records {
		if item.RequiresDepositSync == want {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findByRequiringAutoReturnCheck(want bool) []*account.Record {
	var res []*account.Record
	for _, item := range s.records {
		if item.RequiresAutoReturnCheck == want {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterByType(items []*account.Record, accountType commonpb.AccountType) []*account.Record {
	var res []*account.Record
	for _, item := range items {
		if item.AccountType == accountType {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterByRelationship(items []*account.Record, relationshipTo string) []*account.Record {
	var res []*account.Record
	for _, item := range items {
		if item.RelationshipTo != nil && *item.RelationshipTo == relationshipTo {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) groupByRelationship(items []*account.Record) map[string][]*account.Record {
	res := make(map[string][]*account.Record)
	for _, item := range items {
		key := *pointer.StringOrDefault(item.RelationshipTo, "")
		res[key] = append(res[key], item)
	}
	return res
}

// Put implements account.Store.Put
func (s *store) Put(_ context.Context, data *account.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByAuthorityAddress(data.AuthorityAccount)
	if item != nil && !equivalentRecords(item, data) {
		return account.ErrInvalidAccountInfo
	}

	items := s.findByOwnerAddress(data.OwnerAccount)
	for _, item := range items {
		if !equivalentRecords(item, data) &&
			data.AccountType == item.AccountType &&
			data.Index == item.Index &&
			*pointer.StringOrDefault(item.RelationshipTo, "") == *pointer.StringOrDefault(data.RelationshipTo, "") {
			return account.ErrInvalidAccountInfo
		}
	}

	s.last++
	if item := s.find(data); item != nil {
		if !equivalentRecords(item, data) {
			return account.ErrInvalidAccountInfo
		}

		return account.ErrAccountInfoExists
	}

	data.Id = s.last
	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now()
	}

	cloned := data.Clone()
	s.records = append(s.records, &cloned)

	return nil
}

// Update implements account.Store.Update
func (s *store) Update(_ context.Context, data *account.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		item.RequiresDepositSync = data.RequiresDepositSync
		item.DepositsLastSyncedAt = data.DepositsLastSyncedAt

		item.RequiresAutoReturnCheck = data.RequiresAutoReturnCheck

		item.CopyTo(data)

		return nil
	}
	return account.ErrAccountInfoNotFound
}

// GetByTokenAddress implements account.Store.GetByTokenAddress
func (s *store) GetByTokenAddress(_ context.Context, address string) (*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByTokenAddress(address)
	if item == nil {
		return nil, account.ErrAccountInfoNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetByAuthorityAddress implements account.Store.GetByAuthorityAddress
func (s *store) GetByAuthorityAddress(_ context.Context, address string) (*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findByAuthorityAddress(address)
	if item == nil {
		return nil, account.ErrAccountInfoNotFound
	}

	cloned := item.Clone()
	return &cloned, nil
}

// GetLatestByOwnerAddress implements account.Store.GetLatestByOwnerAddress
func (s *store) GetLatestByOwnerAddress(_ context.Context, address string) (map[commonpb.AccountType][]*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(map[commonpb.AccountType][]*account.Record)

	items := s.findByOwnerAddress(address)
	for _, accountType := range account.AllAccountTypes {
		items := s.filterByType(items, accountType)
		if len(items) == 0 {
			continue
		}

		grouped := s.groupByRelationship(items)

		for _, group := range grouped {
			sorted := ByIndex(group)
			sort.Sort(sorted)

			cloned := group[len(group)-1].Clone()

			res[accountType] = append(res[accountType], &cloned)
		}
	}

	if len(res) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}

	return res, nil
}

// GetLatestByOwnerAddressAndType implements account.Store.GetLatestByOwnerAddressAndType
func (s *store) GetLatestByOwnerAddressAndType(ctx context.Context, address string, accountType commonpb.AccountType) (*account.Record, error) {
	if accountType == commonpb.AccountType_RELATIONSHIP {
		return nil, errors.New("relationship account not supported")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByOwnerAddress(address)
	items = s.filterByType(items, accountType)

	if len(items) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}

	sorted := ByIndex(items)
	sort.Sort(sorted)

	cloned := items[len(items)-1].Clone()
	return &cloned, nil
}

// GetRelationshipByOwnerAddress implements account.Store.GetRelationshipByOwnerAddress
func (s *store) GetRelationshipByOwnerAddress(ctx context.Context, address, relationshipTo string) (*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByOwnerAddress(address)
	items = s.filterByType(items, commonpb.AccountType_RELATIONSHIP)
	items = s.filterByRelationship(items, relationshipTo)

	if len(items) == 0 {
		return nil, account.ErrAccountInfoNotFound
	} else if len(items) > 1 {
		return nil, errors.New("unexpected number of relationship accounts")
	}

	cloned := items[0].Clone()
	return &cloned, nil
}

// GetPrioritizedRequiringDepositSync implements account.Store.GetPrioritizedRequiringDepositSync
func (s *store) GetPrioritizedRequiringDepositSync(_ context.Context, limit uint64) ([]*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByRequiringDepositSync(true)

	if len(items) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}

	sorted := ByDepositsLastSyncedAt(items)
	sort.Sort(sorted)

	var res []*account.Record
	for _, item := range sorted {
		cloned := item.Clone()
		res = append(res, &cloned)
	}

	if len(res) > int(limit) {
		return res[:limit], nil
	}
	return res, nil
}

// CountRequiringDepositSync implements account.Store.CountRequiringDepositSync
func (s *store) CountRequiringDepositSync(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByRequiringDepositSync(true)
	return uint64(len(items)), nil
}

// GetPrioritizedRequiringAutoReturnCheck implements account.Store.GetPrioritizedRequiringAutoReturnCheck
func (s *store) GetPrioritizedRequiringAutoReturnCheck(ctx context.Context, minAge time.Duration, limit uint64) ([]*account.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByRequiringAutoReturnCheck(true)

	var res []*account.Record
	for _, item := range items {
		if time.Since(item.CreatedAt) <= minAge {
			continue
		}

		cloned := item.Clone()
		res = append(res, &cloned)
	}

	if len(res) == 0 {
		return nil, account.ErrAccountInfoNotFound
	}

	sorted := ByCreatedAt(res)
	sort.Sort(sorted)

	if len(res) > int(limit) {
		return res[:limit], nil
	}
	return res, nil
}

// CountRequiringAutoReturnCheck implements account.Store.CountRequiringAutoReturnCheck
func (s *store) CountRequiringAutoReturnCheck(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByRequiringAutoReturnCheck(true)
	return uint64(len(items)), nil
}

func cloneRecords(items []*account.Record) []*account.Record { //nolint:unused
	res := make([]*account.Record, len(items))

	for i, item := range items {
		cloned := item.Clone()
		res[i] = &cloned
	}

	return res
}

func equivalentRecords(obj1, obj2 *account.Record) bool {
	if obj1.OwnerAccount != obj2.OwnerAccount {
		return false
	}

	if obj1.AuthorityAccount != obj2.AuthorityAccount {
		return false
	}

	if obj1.TokenAccount != obj2.TokenAccount {
		return false
	}

	if obj1.Index != obj2.Index {
		return false
	}

	if obj1.AccountType != obj2.AccountType {
		return false
	}

	if *pointer.StringOrDefault(obj1.RelationshipTo, "") != *pointer.StringOrDefault(obj2.RelationshipTo, "") {
		return false
	}

	return true
}
