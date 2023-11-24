package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

type store struct {
	mu      sync.Mutex
	records []*intent.Record
	last    uint64
}

type ById []*intent.Record

func (a ById) Len() int           { return len(a) }
func (a ById) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ById) Less(i, j int) bool { return a[i].Id < a[j].Id }

type ByCreatedAt []*intent.Record

func (a ByCreatedAt) Len() int           { return len(a) }
func (a ByCreatedAt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByCreatedAt) Less(i, j int) bool { return a[i].CreatedAt.Before(a[j].CreatedAt) }

func New() intent.Store {
	return &store{
		records: make([]*intent.Record, 0),
		last:    0,
	}
}

func (s *store) reset() {
	s.mu.Lock()
	s.records = make([]*intent.Record, 0)
	s.last = 0
	s.mu.Unlock()
}

func (s *store) find(data *intent.Record) *intent.Record {
	for _, item := range s.records {
		if item.Id == data.Id {
			return item
		}
		if item.IntentId == data.IntentId {
			return item
		}
	}
	return nil
}

func (s *store) findIntent(intentID string) *intent.Record {
	for _, item := range s.records {
		if item.IntentId == intentID {
			return item
		}
	}
	return nil
}

func (s *store) findByState(state intent.State) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.State == state {
			res = append(res, item)
			continue
		}
	}
	return res
}

func (s *store) findByOwner(owner string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.InitiatorOwnerAccount == owner {
			res = append(res, item)
			continue
		}

		if item.SendPrivatePaymentMetadata != nil && item.SendPrivatePaymentMetadata.DestinationOwnerAccount == owner {
			res = append(res, item)
			continue
		}

		if item.SendPublicPaymentMetadata != nil && item.SendPublicPaymentMetadata.DestinationOwnerAccount == owner {
			res = append(res, item)
			continue
		}

		if item.ExternalDepositMetadata != nil && item.ExternalDepositMetadata.DestinationOwnerAccount == owner {
			res = append(res, item)
			continue
		}
	}

	return res
}

func (s *store) findByDestination(destination string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		switch item.IntentType {
		case intent.LegacyPayment:
			if item.MoneyTransferMetadata.Destination == destination {
				res = append(res, item)
			}
		case intent.SendPrivatePayment:
			if item.SendPrivatePaymentMetadata.DestinationTokenAccount == destination {
				res = append(res, item)
			}
		case intent.ExternalDeposit:
			if item.ExternalDepositMetadata.DestinationTokenAccount == destination {
				res = append(res, item)
			}
		case intent.SendPublicPayment:
			if item.SendPublicPaymentMetadata.DestinationTokenAccount == destination {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findBySource(source string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		switch item.IntentType {
		case intent.LegacyPayment:
			if item.MoneyTransferMetadata.Source == source {
				res = append(res, item)
			}
		case intent.ReceivePaymentsPrivately:
			if item.ReceivePaymentsPrivatelyMetadata.Source == source {
				res = append(res, item)
			}
		case intent.ReceivePaymentsPublicly:
			if item.ReceivePaymentsPubliclyMetadata.Source == source {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findByInitiatorPhoneNumberSinceTimestamp(phoneNumber string, since time.Time) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.CreatedAt.Before(since) {
			continue
		}

		if item.InitiatorPhoneNumber != nil && *item.InitiatorPhoneNumber == phoneNumber {
			res = append(res, item)
		}
	}
	return res
}

func (s *store) findForAntispam(intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.IntentType != intentType {
			continue
		}

		if item.InitiatorPhoneNumber == nil || *item.InitiatorPhoneNumber != phoneNumber {
			continue
		}

		if item.CreatedAt.Before(since) {
			continue
		}

		for _, state := range states {
			if item.State == state {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findOwnerInteractionsForAntispam(sourceOwner, destinationOwner string, states []intent.State, since time.Time) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.InitiatorOwnerAccount != sourceOwner {
			continue
		}

		var destinationOwnerToCheck string
		switch item.IntentType {
		case intent.SendPrivatePayment:
			destinationOwnerToCheck = item.SendPrivatePaymentMetadata.DestinationOwnerAccount
		case intent.SendPublicPayment:
			destinationOwnerToCheck = item.SendPublicPaymentMetadata.DestinationOwnerAccount
		default:
			continue
		}

		if destinationOwnerToCheck != destinationOwner {
			continue
		}

		if item.CreatedAt.Before(since) {
			continue
		}

		for _, state := range states {
			if item.State == state {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) findByInitiatorAndType(intentType intent.Type, owner string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.IntentType != intentType {
			continue
		}

		if item.InitiatorOwnerAccount != owner {
			continue
		}

		res = append(res, item)
	}
	return res
}

func (s *store) findPrePrivacy2022ByTokenAddress(address string) []*intent.Record {
	res := make([]*intent.Record, 0)
	for _, item := range s.records {
		if item.MoneyTransferMetadata != nil {
			if item.MoneyTransferMetadata.Source == address {
				res = append(res, item)
				continue
			}
			if item.MoneyTransferMetadata.Destination == address {
				res = append(res, item)
				continue
			}
		}

		if item.AccountManagementMetadata != nil && item.AccountManagementMetadata.TokenAccount == address {
			res = append(res, item)
			continue
		}
	}
	return res
}

func sumQuarkAmount(items []*intent.Record) uint64 {
	var value uint64
	for _, item := range items {
		if item.SendPrivatePaymentMetadata != nil {
			value += item.SendPrivatePaymentMetadata.Quantity
		}
		if item.ReceivePaymentsPrivatelyMetadata != nil {
			value += item.ReceivePaymentsPrivatelyMetadata.Quantity
		}
	}
	return value
}

func sumUsdMarketValue(items []*intent.Record) float64 {
	var value float64
	for _, item := range items {
		if item.SendPrivatePaymentMetadata != nil {
			value += item.SendPrivatePaymentMetadata.UsdMarketValue
		}
		if item.ReceivePaymentsPrivatelyMetadata != nil {
			value += item.ReceivePaymentsPrivatelyMetadata.UsdMarketValue
		}
	}
	return value
}

func (s *store) filter(items []*intent.Record, cursor query.Cursor, limit uint64, direction query.Ordering) []*intent.Record {
	var start uint64

	start = 0
	if direction == query.Descending {
		start = s.last + 1
	}
	if len(cursor) > 0 {
		start = cursor.ToUint64()
	}

	var res []*intent.Record
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

func (s *store) filterByState(items []*intent.Record, include bool, states ...intent.State) []*intent.Record {
	var res []*intent.Record

	for _, item := range items {
		for _, state := range states {
			if item.State == state && include {
				res = append(res, item)
			} else if item.State != state && !include {
				res = append(res, item)
			}
		}
	}

	return res
}

func (s *store) filterByType(items []*intent.Record, intentType intent.Type) []*intent.Record {
	var res []*intent.Record

	for _, item := range items {
		if item.IntentType == intentType {
			res = append(res, item)
		}
	}

	return res
}

func (s *store) filterByDeposit(items []*intent.Record) []*intent.Record {
	var res []*intent.Record

	for _, item := range items {
		if item.IntentType != intent.ReceivePaymentsPrivately {
			continue
		}

		if !item.ReceivePaymentsPrivatelyMetadata.IsDeposit {
			continue
		}

		res = append(res, item)
	}

	return res
}

func (s *store) filterByRemoteSendFlag(items []*intent.Record, want bool) []*intent.Record {
	var res []*intent.Record
	for _, item := range items {
		switch item.IntentType {
		case intent.SendPrivatePayment:
			if item.SendPrivatePaymentMetadata.IsRemoteSend == want {
				res = append(res, item)
			}
		case intent.ReceivePaymentsPublicly:
			if item.ReceivePaymentsPubliclyMetadata.IsRemoteSend == want {
				res = append(res, item)
			}
		}
	}
	return res
}

func (s *store) Save(ctx context.Context, data *intent.Record) error {
	if err := data.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.last++
	if item := s.find(data); item != nil {
		// Only update state
		item.State = data.State
	} else {
		if data.Id == 0 {
			data.Id = s.last
		}
		if data.CreatedAt.IsZero() {
			data.CreatedAt = time.Now()
		}
		c := data.Clone()
		s.records = append(s.records, &c)
	}

	return nil
}

func (s *store) Get(ctx context.Context, intentID string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item := s.findIntent(intentID); item != nil {
		return item, nil
	}

	return nil, intent.ErrIntentNotFound
}

func (s *store) GetLatestByInitiatorAndType(ctx context.Context, intentType intent.Type, owner string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByInitiatorAndType(intentType, owner)
	if len(items) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	latest := items[0]
	for _, item := range items {
		if item.CreatedAt.After(latest.CreatedAt) {
			latest = item
		}
	}

	return latest, nil
}

func (s *store) GetAllByOwner(ctx context.Context, owner string, cursor query.Cursor, limit uint64, direction query.Ordering) ([]*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if items := s.findByOwner(owner); len(items) > 0 {
		res := s.filter(items, cursor, limit, direction)

		if len(res) == 0 {
			return nil, intent.ErrIntentNotFound
		}

		return res, nil
	}

	return nil, intent.ErrIntentNotFound
}

func (s *store) CountForAntispam(ctx context.Context, intentType intent.Type, phoneNumber string, states []intent.State, since time.Time) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findForAntispam(intentType, phoneNumber, states, since)
	return uint64(len(items)), nil
}

func (s *store) CountOwnerInteractionsForAntispam(ctx context.Context, sourceOwner, destinationOwner string, states []intent.State, since time.Time) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findOwnerInteractionsForAntispam(sourceOwner, destinationOwner, states, since)
	return uint64(len(items)), nil
}

func (s *store) GetTransactedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByInitiatorPhoneNumberSinceTimestamp(phoneNumber, since)
	items = s.filterByState(items, false, intent.StateRevoked)
	items = s.filterByType(items, intent.SendPrivatePayment)
	return sumQuarkAmount(items), sumUsdMarketValue(items), nil
}

func (s *store) GetDepositedAmountForAntiMoneyLaundering(ctx context.Context, phoneNumber string, since time.Time) (uint64, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByInitiatorPhoneNumberSinceTimestamp(phoneNumber, since)
	items = s.filterByState(items, false, intent.StateRevoked)
	items = s.filterByDeposit(items)
	return sumQuarkAmount(items), sumUsdMarketValue(items), nil
}

func (s *store) GetNetBalanceFromPrePrivacy2022Intents(ctx context.Context, account string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res int64

	items := s.findPrePrivacy2022ByTokenAddress(account)
	for _, item := range items {
		if item.IntentType != intent.LegacyPayment {
			continue
		}

		if item.State == intent.StateUnknown || item.State == intent.StateRevoked {
			continue
		}

		if item.MoneyTransferMetadata.Source == account {
			res -= int64(item.MoneyTransferMetadata.Quantity)
		}

		if item.MoneyTransferMetadata.Destination == account {
			res += int64(item.MoneyTransferMetadata.Quantity)
		}
	}

	return res, nil
}

func (s *store) GetLatestSaveRecentRootIntentForTreasury(ctx context.Context, treasury string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var latest *intent.Record
	for _, record := range s.records {
		if record.IntentType != intent.SaveRecentRoot {
			continue
		}

		if record.SaveRecentRootMetadata.TreasuryPool != treasury {
			continue
		}

		if latest == nil || latest.Id < record.Id {
			latest = record
		}
	}

	if latest == nil {
		return nil, intent.ErrIntentNotFound
	}
	cloned := latest.Clone()
	return &cloned, nil
}

func (s *store) GetOriginalGiftCardIssuedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findByDestination(giftCardVault)
	items = s.filterByType(items, intent.SendPrivatePayment)
	items = s.filterByState(items, false, intent.StateRevoked)
	items = s.filterByRemoteSendFlag(items, true)

	if len(items) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	if len(items) > 1 {
		return nil, intent.ErrMultilpeIntentsFound
	}

	cloned := items[0].Clone()
	return &cloned, nil
}

func (s *store) GetGiftCardClaimedIntent(ctx context.Context, giftCardVault string) (*intent.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.findBySource(giftCardVault)
	items = s.filterByType(items, intent.ReceivePaymentsPublicly)
	items = s.filterByState(items, false, intent.StateRevoked)
	items = s.filterByRemoteSendFlag(items, true)

	if len(items) == 0 {
		return nil, intent.ErrIntentNotFound
	}

	if len(items) > 1 {
		return nil, intent.ErrMultilpeIntentsFound
	}

	cloned := items[0].Clone()
	return &cloned, nil
}
