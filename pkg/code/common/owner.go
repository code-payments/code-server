package common

import (
	"context"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
)

var (
	ErrOwnerNotFound = errors.New("owner account not found")
)

type OwnerType uint8

const (
	OwnerTypeUnknown OwnerType = iota
	OwnerTypeUser12Words
	OwnerTypeRemoteSendGiftCard
)

type OwnerManagementState uint8

const (
	OwnerManagementStateUnknown OwnerManagementState = iota
	OwnerManagementStateCodeAccount
	OwnerManagementStateNotFound
	OwnerManagementStateUnlocked
)

type OwnerMetadata struct {
	Type    OwnerType
	Account *Account
	State   OwnerManagementState
}

// GetOwnerMetadata gets metadata about an owner account
//
// todo: assumes the core mint accounts are always opened first
func GetOwnerMetadata(ctx context.Context, data code_data.Provider, owner *Account) (*OwnerMetadata, error) {
	mtdt := &OwnerMetadata{
		Account: owner,
	}

	// Is the owner account a remote send gift card?
	_, err := data.GetLatestAccountInfoByOwnerAddressAndType(ctx, owner.publicKey.ToBase58(), commonpb.AccountType_REMOTE_SEND_GIFT_CARD)
	if err == nil {
		mtdt.Type = OwnerTypeRemoteSendGiftCard
	} else if err != account.ErrAccountInfoNotFound {
		return nil, err
	}

	if mtdt.Type == OwnerTypeUnknown {
		// Is the owner account a user 12 words? We know by detecting the presence of a core mint primary account.
		accountInfoRecordsByMint, err := data.GetLatestAccountInfoByOwnerAddressAndType(ctx, owner.publicKey.ToBase58(), commonpb.AccountType_PRIMARY)
		if err == nil {
			_, ok := accountInfoRecordsByMint[CoreMintAccount.PublicKey().ToBase58()]
			if ok {
				mtdt.Type = OwnerTypeUser12Words
			}
		} else if err != account.ErrAccountInfoNotFound {
			return nil, err
		}
	}

	// No other cases for an owner account, so error out
	if mtdt.Type == OwnerTypeUnknown {
		return nil, ErrOwnerNotFound
	}

	state, err := GetOwnerManagementState(ctx, data, owner)
	if err != nil {
		return nil, err
	}
	mtdt.State = state

	return mtdt, nil
}

// GetOwnerManagementState returns an aggregate management state for an owner
// account based on the set of sub accounts it owns.
//
// todo: Needs tests here, but most already exist in account service
func GetOwnerManagementState(ctx context.Context, data code_data.Provider, owner *Account) (OwnerManagementState, error) {
	recordsByMintAndType, err := GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		return OwnerManagementStateUnknown, err
	}

	// Has an account ever been opened with the owner? If not, the owner is not a Code account.
	// SubmitIntent guarantees all accounts are opened, so there's no need to do anything more
	// than an empty check.
	if len(recordsByMintAndType) == 0 {
		return OwnerManagementStateNotFound, nil
	}

	// Are all opened accounts managed by Code? If not, the owner is not a Code account.
	for _, recordsByType := range recordsByMintAndType {
		for _, recordsList := range recordsByType {
			for _, records := range recordsList {
				if records.IsTimelock() && !records.IsManagedByCode(ctx) {
					return OwnerManagementStateUnlocked, nil
				}
			}
		}
	}

	return OwnerManagementStateCodeAccount, nil
}

// GetLatestTokenAccountRecordsForOwner gets DB records for the latest set of
// token accounts for an owner account.
func GetLatestTokenAccountRecordsForOwner(ctx context.Context, data code_data.Provider, owner *Account) (map[string]map[commonpb.AccountType][]*AccountRecords, error) {
	res := make(map[string]map[commonpb.AccountType][]*AccountRecords)

	infoRecordsByMintAndType, err := data.GetLatestAccountInfosByOwnerAddress(ctx, owner.publicKey.ToBase58())
	if err != account.ErrAccountInfoNotFound && err != nil {
		return nil, err
	}

	if len(infoRecordsByMintAndType) == 0 {
		return res, nil
	}

	var timelockAccounts []string
	for _, infoRecordsByType := range infoRecordsByMintAndType {
		for _, infoRecords := range infoRecordsByType {
			for _, infoRecord := range infoRecords {
				if infoRecord.IsTimelock() {
					timelockAccounts = append(timelockAccounts, infoRecord.TokenAccount)
				}
			}
		}
	}

	timelockRecordsByVault, err := data.GetTimelockByVaultBatch(ctx, timelockAccounts...)
	if err != nil {
		return nil, err
	}

	for _, infoRecordsByType := range infoRecordsByMintAndType {
		for _, infoRecords := range infoRecordsByType {
			for _, infoRecord := range infoRecords {
				var timelockRecord *timelock.Record
				var ok bool
				if infoRecord.IsTimelock() {
					timelockRecord, ok = timelockRecordsByVault[infoRecord.TokenAccount]
					if !ok {
						return nil, errors.New("timelock record unexpectedly doesn't exist")
					}
				}

				// Filter out account records for accounts that have completed their
				// full lifecycle
				//
				// todo: This needs tests
				switch infoRecord.AccountType {
				case commonpb.AccountType_POOL:
					if timelockRecord.IsClosed() {
						continue
					}
				}

				if _, ok := res[infoRecord.MintAccount]; !ok {
					res[infoRecord.MintAccount] = make(map[commonpb.AccountType][]*AccountRecords)
				}

				res[infoRecord.MintAccount][infoRecord.AccountType] = append(res[infoRecord.MintAccount][infoRecord.AccountType], &AccountRecords{
					General:  infoRecord,
					Timelock: timelockRecord,
				})
			}
		}
	}

	return res, nil
}

// GetLatestCodeTimelockAccountRecordsForOwner is a utility wrapper over GetLatestTokenAccountRecordsForOwner
// that filters for Code Timelock accounts.
func GetLatestCodeTimelockAccountRecordsForOwner(ctx context.Context, data code_data.Provider, owner *Account) (map[string]map[commonpb.AccountType][]*AccountRecords, error) {
	res := make(map[string]map[commonpb.AccountType][]*AccountRecords)

	recordsByMintAndType, err := GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		return nil, err
	}

	for _, recordsByType := range recordsByMintAndType {
		for _, recordsList := range recordsByType {
			for _, records := range recordsList {
				if records.IsTimelock() {
					if _, ok := res[records.General.MintAccount]; !ok {
						res[records.General.MintAccount] = make(map[commonpb.AccountType][]*AccountRecords)
					}

					res[records.General.MintAccount][records.General.AccountType] = append(res[records.General.MintAccount][records.General.AccountType], records)
				}
			}
		}
	}

	return res, nil
}

func (t OwnerType) String() string {
	switch t {
	case OwnerTypeUnknown:
		return "unknown"
	case OwnerTypeUser12Words:
		return "user_12_words"
	case OwnerTypeRemoteSendGiftCard:
		return "remote_send_gift_card"
	}
	return "unknown"
}

func (t OwnerManagementState) String() string {
	switch t {
	case OwnerManagementStateUnknown:
		return "unknown"
	case OwnerManagementStateNotFound:
		return "not_found"
	case OwnerManagementStateUnlocked:
		return "unlocked"
	case OwnerManagementStateCodeAccount:
		return "code_account"
	}
	return "unknown"
}
