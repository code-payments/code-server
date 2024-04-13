package memory

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/pointer"
)

type store struct {
	mu      sync.Mutex
	records []*fulfillment.Record
	last    uint64
}

type ById []*fulfillment.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

func New() fulfillment.Store {
	return &store{
		records: make([]*fulfillment.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*fulfillment.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

func (s *store) findById(id uint64) *fulfillment.Record {
	for _, item := range s.records {
		if item.Id == id {
			return item
		}
	}
	return nil
}

func (s *store) findBySignature(sig string) *fulfillment.Record {
	for _, item := range s.records {
		if item.Signature != nil && *item.Signature == sig {
			return item
		}
	}
	return nil
}

func (s *store) findByIntent(intent string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.Intent == intent {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByAction(intent string, actionId uint32) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.Intent == intent && item.ActionId == actionId {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByIntentAndState(intent string, state fulfillment.State) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.Intent == intent && item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByState(state fulfillment.State) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByStateAndAddress(state fulfillment.State, address string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != state {
			continue
		}

		if item.Source == address {
			res = append(res, item)
			continue
		}

		if item.Destination != nil && *item.Destination == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByStateAndAddressAsSource(state fulfillment.State, address string) []*fulfillment.Record { //nolint:unused
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != state {
			continue
		}

		if item.Source == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByTypeStateAndAddress(fulfillmentType fulfillment.Type, state fulfillment.State, address string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.FulfillmentType != fulfillmentType {
			continue
		}

		if item.State != state {
			continue
		}

		if item.Source == address {
			res = append(res, item)
			continue
		}

		if item.Destination != nil && *item.Destination == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByTypeStateAndAddressAsSource(fulfillmentType fulfillment.Type, state fulfillment.State, address string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.FulfillmentType != fulfillmentType {
			continue
		}

		if item.State != state {
			continue
		}

		if item.Source == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByTypeActionAndState(intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.Intent != intentId {
			continue
		}

		if item.ActionId != actionId {
			continue
		}

		if item.FulfillmentType != fulfillmentType {
			continue
		}

		if item.State != state {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) findScheduableByAddress(address string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != fulfillment.StateUnknown && item.State != fulfillment.StatePending {
			continue
		}

		if item.Source == address || (item.Destination != nil && *item.Destination == address) {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findScheduableBySource(source string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != fulfillment.StateUnknown && item.State != fulfillment.StatePending {
			continue
		}

		if item.Source == source {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findScheduableByDestination(destinaion string) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != fulfillment.StateUnknown && item.State != fulfillment.StatePending {
			continue
		}

		if item.Destination != nil && *item.Destination == destinaion {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findScheduableByType(fulfillmentType fulfillment.Type) []*fulfillment.Record {
	res := make([]*fulfillment.Record, 0)
	for _, item := range s.records {
		if item.State != fulfillment.StateUnknown && item.State != fulfillment.StatePending {
			continue
		}

		if item.FulfillmentType == fulfillmentType {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) filter(items []*fulfillment.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*fulfillment.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*fulfillment.Record
	for _, item := range items {
		if item.Id > start && direction == query.Ascending {
			res = append(res, item)
		}
		if item.Id < start && direction == query.Descending {
			res = append(res, item)
		}
	}

	if direction == query.Descending {
		sort.Sort(sort.Reverse(ById(res)))
	}

	if len(res) >= int(limit) {
		return res[:limit]
	}

	return res
}

func (s *store) filterByType(items []*fulfillment.Record, fulfillmentType fulfillment.Type) []*fulfillment.Record {
	var res []*fulfillment.Record
	for _, item := range items {
		if item.FulfillmentType == fulfillmentType {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterDisabledActiveScheduling(items []*fulfillment.Record) []*fulfillment.Record {
	var res []*fulfillment.Record
	for _, item := range items {
		if !item.DisableActiveScheduling {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) filterScheduledAfter(items []*fulfillment.Record, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) []*fulfillment.Record {
	var res []*fulfillment.Record
	for _, item := range items {
		if item.IntentOrderingIndex > intentOrderingIndex {
			res = append(res, item)
			continue
		}

		if item.IntentOrderingIndex == intentOrderingIndex && item.ActionOrderingIndex > actionOrderingIndex {
			res = append(res, item)
			continue
		}

		if item.IntentOrderingIndex == intentOrderingIndex && item.ActionOrderingIndex == actionOrderingIndex && item.FulfillmentOrderingIndex > fulfillmentOrderingIndex {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) Count(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return uint64(len(s.records)), nil
}

func (s *store) CountByState(ctx context.Context, state fulfillment.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByState(state)
	return uint64(len(res)), nil
}

func (s *store) CountByStateGroupedByType(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(state)

	res := make(map[fulfillment.Type]uint64)
	for _, item := range items {
		res[item.FulfillmentType] += 1
	}
	return res, nil
}

func (s *store) CountForMetrics(ctx context.Context, state fulfillment.State) (map[fulfillment.Type]uint64, error) {
	return nil, errors.New("not implemented")
}

func (s *store) CountByStateAndAddress(ctx context.Context, state fulfillment.State, address string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByStateAndAddress(state, address)
	return uint64(len(res)), nil
}

func (s *store) CountByTypeStateAndAddressAsSource(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByTypeStateAndAddressAsSource(fulfillmentType, state, address)
	return uint64(len(res)), nil
}

func (s *store) CountByTypeStateAndAddress(ctx context.Context, fulfillmentType fulfillment.Type, state fulfillment.State, address string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByTypeStateAndAddress(fulfillmentType, state, address)
	return uint64(len(res)), nil
}

func (s *store) CountByIntentAndState(ctx context.Context, intent string, state fulfillment.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByIntentAndState(intent, state)
	return uint64(len(res)), nil
}

func (s *store) CountByTypeActionAndState(ctx context.Context, intentId string, actionId uint32, fulfillmentType fulfillment.Type, state fulfillment.State) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByTypeActionAndState(intentId, actionId, fulfillmentType, state)
	return uint64(len(res)), nil
}

func (s *store) CountByIntent(ctx context.Context, intent string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByIntent(intent)
	return uint64(len(res)), nil
}

func (s *store) CountPendingByType(ctx context.Context) (map[fulfillment.Type]uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByState(fulfillment.StatePending)

	res := make(map[fulfillment.Type]uint64)
	for _, item := range items {
		res[item.FulfillmentType] += 1
	}
	return res, nil
}

func (s *store) PutAll(ctx context.Context, records ...*fulfillment.Record) error {
	signatures := make(map[string]struct{})
	for _, data := range records {
		if err := data.Validate(); err != nil {
			return err
		}

		if data.Id != 0 {
			return fulfillment.ErrFulfillmentExists
		}

		if data.Signature != nil {
			_, ok := signatures[*data.Signature]
			if ok {
				return fulfillment.ErrFulfillmentExists
			}
			signatures[*data.Signature] = struct{}{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, data := range records {
		if data.Signature != nil {
			item := s.findBySignature(*data.Signature)
			if item != nil {
				return fulfillment.ErrFulfillmentExists
			}
		}
	}

	for _, data := range records {
		s.last++

		data.Id = s.last
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}

		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Update(ctx context.Context, data *fulfillment.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	if data.Id == 0 {
		return fulfillment.ErrFulfillmentNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++

	item := s.findById(data.Id)
	if item == nil {
		return fulfillment.ErrFulfillmentNotFound
	}

	item.Signature = pointer.StringCopy(data.Signature)
	item.Nonce = pointer.StringCopy(data.Nonce)
	item.Blockhash = pointer.StringCopy(data.Blockhash)
	item.Data = data.Data
	item.State = data.State

	if item.FulfillmentType == fulfillment.CloseDormantTimelockAccount {
		item.IntentOrderingIndex = data.IntentOrderingIndex
		item.ActionOrderingIndex = data.ActionOrderingIndex
		item.FulfillmentOrderingIndex = data.FulfillmentOrderingIndex
	}

	return nil
}

func (s *store) MarkAsActivelyScheduled(ctx context.Context, id uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.findById(id)
	if item == nil {
		return fulfillment.ErrFulfillmentNotFound
	}

	item.DisableActiveScheduling = false

	return nil
}

func (s *store) ActivelyScheduleTreasuryAdvances(ctx context.Context, treasury string, intentOrderingIndex uint64, limit int) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var updateCount uint64
	for _, data := range s.records {
		if !data.DisableActiveScheduling {
			continue
		}

		if data.State != fulfillment.StateUnknown {
			continue
		}

		if data.FulfillmentType != fulfillment.TransferWithCommitment {
			continue
		}

		if data.Source != treasury {
			continue
		}

		if data.IntentOrderingIndex >= intentOrderingIndex {
			continue
		}

		data.DisableActiveScheduling = false

		updateCount += 1
		if updateCount >= uint64(limit) {
			return updateCount, nil
		}
	}

	return updateCount, nil
}

func (s *store) GetById(ctx context.Context, id uint64) (*fulfillment.Record, error) {
	if id == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findById(id); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}

	return nil, fulfillment.ErrFulfillmentNotFound
}

func (s *store) GetBySignature(ctx context.Context, sig string) (*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findBySignature(sig); item != nil {
		cloned := item.Clone()
		return &cloned, nil
	}

	return nil, fulfillment.ErrFulfillmentNotFound
}

func (s *store) GetAllByState(ctx context.Context, state fulfillment.State, includeDisabledActiveScheduling bool, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByState(state); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if !includeDisabledActiveScheduling {
			res = s.filterDisabledActiveScheduling(res)
		}

		if len(res) == 0 {
			return nil, fulfillment.ErrFulfillmentNotFound
		}

		return cloneAll(res), nil
	}

	return nil, fulfillment.ErrFulfillmentNotFound
}

func (s *store) GetAllByIntent(ctx context.Context, intent string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByIntent(intent); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, fulfillment.ErrFulfillmentNotFound
		}

		return cloneAll(res), nil
	}

	return nil, fulfillment.ErrFulfillmentNotFound
}

// GetAllByAction returns all fulfillment records for a given action
func (s *store) GetAllByAction(ctx context.Context, intentId string, actionId uint32) ([]*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByAction(intentId, actionId)
	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return cloneAll(res), nil
}

func (s *store) GetAllByTypeAndAction(ctx context.Context, fulfillmentType fulfillment.Type, intentId string, actionId uint32) ([]*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := s.findByAction(intentId, actionId)
	res = s.filterByType(res, fulfillmentType)
	if len(res) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	return cloneAll(res), nil
}

func (s *store) GetFirstSchedulableByAddressAsSource(ctx context.Context, address string) (*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findScheduableBySource(address); len(items) > 0 {
		sorted := fulfillment.BySchedulingOrder(items)
		sort.Sort(sorted)

		cloned := sorted[0].Clone()
		return &cloned, nil
	}
	return nil, fulfillment.ErrFulfillmentNotFound
}
func (s *store) GetFirstSchedulableByAddressAsDestination(ctx context.Context, address string) (*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findScheduableByDestination(address); len(items) > 0 {
		sorted := fulfillment.BySchedulingOrder(items)
		sort.Sort(sorted)

		cloned := sorted[0].Clone()
		return &cloned, nil
	}
	return nil, fulfillment.ErrFulfillmentNotFound
}

func (s *store) GetFirstSchedulableByType(ctx context.Context, fulfillmentType fulfillment.Type) (*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findScheduableByType(fulfillmentType); len(items) > 0 {
		sorted := fulfillment.BySchedulingOrder(items)
		sort.Sort(sorted)

		cloned := sorted[0].Clone()
		return &cloned, nil
	}
	return nil, fulfillment.ErrFulfillmentNotFound
}

func (s *store) GetNextSchedulableByAddress(ctx context.Context, address string, intentOrderingIndex uint64, actionOrderingIndex, fulfillmentOrderingIndex uint32) (*fulfillment.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findScheduableByAddress(address)
	items = s.filterScheduledAfter(items, intentOrderingIndex, actionOrderingIndex, fulfillmentOrderingIndex)

	sorted := fulfillment.BySchedulingOrder(items)
	sort.Sort(sorted)

	if len(sorted) == 0 {
		return nil, fulfillment.ErrFulfillmentNotFound
	}

	cloned := sorted[0].Clone()
	return &cloned, nil
}

func cloneAll(items []*fulfillment.Record) []*fulfillment.Record {
	var res []*fulfillment.Record
	for _, item := range items {
		cloned := item.Clone()
		res = append(res, &cloned)
	}
	return res
}
