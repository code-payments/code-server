package account

import (
	"time"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
)

var AllAccountTypes = []commonpb.AccountType{
	commonpb.AccountType_PRIMARY,
	commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
	commonpb.AccountType_POOL,
	commonpb.AccountType_SWAP,
}

type Record struct {
	Id uint64

	OwnerAccount     string
	AuthorityAccount string
	TokenAccount     string
	MintAccount      string

	AccountType commonpb.AccountType
	Index       uint64

	RequiresDepositSync  bool
	DepositsLastSyncedAt time.Time

	RequiresAutoReturnCheck bool

	RequiresSwapRetry bool
	LastSwapRetryAt   time.Time

	CreatedAt time.Time
}

func (r *Record) Clone() Record {
	return Record{
		Id:                      r.Id,
		OwnerAccount:            r.OwnerAccount,
		AuthorityAccount:        r.AuthorityAccount,
		TokenAccount:            r.TokenAccount,
		MintAccount:             r.MintAccount,
		AccountType:             r.AccountType,
		Index:                   r.Index,
		RequiresDepositSync:     r.RequiresDepositSync,
		DepositsLastSyncedAt:    r.DepositsLastSyncedAt,
		RequiresAutoReturnCheck: r.RequiresAutoReturnCheck,
		RequiresSwapRetry:       r.RequiresSwapRetry,
		LastSwapRetryAt:         r.LastSwapRetryAt,
		CreatedAt:               r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.OwnerAccount = r.OwnerAccount
	dst.AuthorityAccount = r.AuthorityAccount
	dst.TokenAccount = r.TokenAccount
	dst.MintAccount = r.MintAccount
	dst.AccountType = r.AccountType
	dst.Index = r.Index
	dst.RequiresDepositSync = r.RequiresDepositSync
	dst.DepositsLastSyncedAt = r.DepositsLastSyncedAt
	dst.RequiresAutoReturnCheck = r.RequiresAutoReturnCheck
	dst.RequiresSwapRetry = r.RequiresSwapRetry
	dst.LastSwapRetryAt = r.LastSwapRetryAt
	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if len(r.OwnerAccount) == 0 {
		return errors.New("owner address is required")
	}

	if len(r.AuthorityAccount) == 0 {
		return errors.New("authority address is required")
	}

	if len(r.TokenAccount) == 0 {
		return errors.New("token address is required")
	}

	if len(r.MintAccount) == 0 {
		return errors.New("mint address is required")
	}

	if r.AccountType == commonpb.AccountType_UNKNOWN {
		return errors.New("account type is required")
	}

	switch r.AccountType {
	case commonpb.AccountType_PRIMARY:
		if r.Index != 0 {
			return errors.New("index must be 0 for primary account")
		}

		if r.OwnerAccount != r.AuthorityAccount {
			return errors.New("owner must be authority of primary account")
		}
	case commonpb.AccountType_REMOTE_SEND_GIFT_CARD:
		if r.Index != 0 {
			return errors.New("index must be 0 for remote send gift card account")
		}

		if r.OwnerAccount != r.AuthorityAccount {
			return errors.New("owner must be authority of remote send gift card account")
		}
	case commonpb.AccountType_POOL:
		if r.OwnerAccount == r.AuthorityAccount {
			return errors.New("owner cannot be authority pool account")
		}
	case commonpb.AccountType_SWAP:
		if r.Index != 0 {
			return errors.New("index must be 0 for swap account")
		}

		if r.OwnerAccount == r.AuthorityAccount {
			return errors.New("owner cannot be authority for swap account")
		}
	default:
		return errors.Errorf("unsupported account type: %s", r.AccountType.String())
	}

	if r.TokenAccount == r.OwnerAccount || r.TokenAccount == r.AuthorityAccount {
		return errors.New("token account cannot be owner or authority of account")
	}

	if r.RequiresAutoReturnCheck && r.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return errors.New("only remote send gift cards can have auto-return checks")
	}

	if r.RequiresSwapRetry && r.AccountType != commonpb.AccountType_SWAP {
		return errors.New("only swap accounts can require swap retries")
	}

	return nil
}

func (r *Record) IsTimelock() bool {
	return r.AccountType != commonpb.AccountType_SWAP
}
