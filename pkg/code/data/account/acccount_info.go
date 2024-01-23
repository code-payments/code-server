package account

import (
	"time"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/pointer"
)

var AllAccountTypes = []commonpb.AccountType{
	commonpb.AccountType_PRIMARY,
	commonpb.AccountType_TEMPORARY_INCOMING,
	commonpb.AccountType_TEMPORARY_OUTGOING,
	commonpb.AccountType_BUCKET_1_KIN,
	commonpb.AccountType_BUCKET_10_KIN,
	commonpb.AccountType_BUCKET_100_KIN,
	commonpb.AccountType_BUCKET_1_000_KIN,
	commonpb.AccountType_BUCKET_10_000_KIN,
	commonpb.AccountType_BUCKET_100_000_KIN,
	commonpb.AccountType_BUCKET_1_000_000_KIN,
	commonpb.AccountType_REMOTE_SEND_GIFT_CARD,
	commonpb.AccountType_RELATIONSHIP,
}

type Record struct {
	Id uint64

	OwnerAccount     string
	AuthorityAccount string
	TokenAccount     string
	MintAccount      string

	AccountType    commonpb.AccountType
	Index          uint64
	RelationshipTo *string

	RequiresDepositSync  bool
	DepositsLastSyncedAt time.Time

	RequiresAutoReturnCheck bool

	CreatedAt time.Time
}

func (r *Record) IsBucket() bool {
	switch r.AccountType {
	case commonpb.AccountType_BUCKET_1_KIN,
		commonpb.AccountType_BUCKET_10_KIN,
		commonpb.AccountType_BUCKET_100_KIN,
		commonpb.AccountType_BUCKET_1_000_KIN,
		commonpb.AccountType_BUCKET_10_000_KIN,
		commonpb.AccountType_BUCKET_100_000_KIN,
		commonpb.AccountType_BUCKET_1_000_000_KIN:
		return true
	}

	return false
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
		RelationshipTo:          pointer.StringCopy(r.RelationshipTo),
		RequiresDepositSync:     r.RequiresDepositSync,
		DepositsLastSyncedAt:    r.DepositsLastSyncedAt,
		RequiresAutoReturnCheck: r.RequiresAutoReturnCheck,
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
	dst.RelationshipTo = pointer.StringCopy(dst.RelationshipTo)
	dst.RequiresDepositSync = r.RequiresDepositSync
	dst.DepositsLastSyncedAt = r.DepositsLastSyncedAt
	dst.RequiresAutoReturnCheck = r.RequiresAutoReturnCheck
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
	case commonpb.AccountType_LEGACY_PRIMARY_2022:
		return errors.New("cannot store legacy primary 2022 account")
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
	case commonpb.AccountType_RELATIONSHIP:
		if r.Index != 0 {
			return errors.New("index must be 0 for relationship account")
		}

		if r.OwnerAccount == r.AuthorityAccount {
			return errors.New("owner cannot be authority for relationship account")
		}

		if r.RelationshipTo == nil || len(*r.RelationshipTo) == 0 {
			return errors.New("relationship metadata required for relationship account")
		}
	case commonpb.AccountType_BUCKET_1_KIN,
		commonpb.AccountType_BUCKET_10_KIN,
		commonpb.AccountType_BUCKET_100_KIN,
		commonpb.AccountType_BUCKET_1_000_KIN,
		commonpb.AccountType_BUCKET_10_000_KIN,
		commonpb.AccountType_BUCKET_100_000_KIN,
		commonpb.AccountType_BUCKET_1_000_000_KIN:

		if r.Index != 0 {
			return errors.New("index must be 0 for bucket account")
		}

		if r.OwnerAccount == r.AuthorityAccount {
			return errors.New("owner cannot be authority for bucket account")
		}
	case commonpb.AccountType_TEMPORARY_INCOMING,
		commonpb.AccountType_TEMPORARY_OUTGOING:
		if r.OwnerAccount == r.AuthorityAccount {
			return errors.New("owner cannot be authority for temporary rotating account")
		}
	default:
		return errors.Errorf("unhandled account type: %s", r.AccountType.String())
	}

	if r.TokenAccount == r.OwnerAccount || r.TokenAccount == r.AuthorityAccount {
		return errors.New("token account cannot be owner or authority of account")
	}

	if r.RequiresAutoReturnCheck && r.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return errors.New("only remote send gift cards can have auto-return checks")
	}

	if r.RelationshipTo != nil && r.AccountType != commonpb.AccountType_RELATIONSHIP {
		return errors.New("only relationship accounts can have a relationship metadata")
	}

	return nil
}
