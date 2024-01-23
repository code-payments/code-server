package common

import (
	"context"

	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/timelock"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
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
	Type               OwnerType
	Account            *Account
	VerificationRecord *phone.Verification
	State              OwnerManagementState
}

// GetOwnerMetadata gets metadata about an owner account
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
		// Is the owner account a user's 12 words that's phone verified?
		//
		// This should be the last thing checked, since it's technically possible
		// today for a malicious user to phone very any owner account type.
		verificationRecord, err := data.GetLatestPhoneVerificationForAccount(ctx, owner.publicKey.ToBase58())
		if err == nil {
			mtdt.Type = OwnerTypeUser12Words
			mtdt.VerificationRecord = verificationRecord
		} else if err != phone.ErrVerificationNotFound {
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
	legacyPrimary2022Records, err := GetLegacyPrimary2022AccountRecordsIfNotMigrated(ctx, data, owner)
	if err != ErrNoPrivacyMigration2022 && err != nil {
		return OwnerManagementStateUnknown, err
	}
	hasLegacyPrimary2022Account := (err == nil)

	recordsByType, err := GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		return OwnerManagementStateUnknown, err
	}

	// Has an account ever been opened with the owner? If not, the owner is not a Code account.
	// SubmitIntent guarantees all accounts are opened, so there's no need to do anything more
	// than an empty check.
	if len(recordsByType) == 0 && !hasLegacyPrimary2022Account {
		return OwnerManagementStateNotFound, nil
	}

	// Are all opened accounts managed by Code? If not, the owner is not a Code account.
	if hasLegacyPrimary2022Account && !legacyPrimary2022Records.IsManagedByCode(ctx) {
		return OwnerManagementStateUnlocked, nil
	}
	for _, batchAccountRecords := range recordsByType {
		for _, accountRecords := range batchAccountRecords {
			if accountRecords.IsTimelock() && !accountRecords.IsManagedByCode(ctx) {
				return OwnerManagementStateUnlocked, nil
			}
		}
	}

	return OwnerManagementStateCodeAccount, nil
}

// GetLatestTokenAccountRecordsForOwner gets DB records for the latest set of
// token accounts for an owner account.
func GetLatestTokenAccountRecordsForOwner(ctx context.Context, data code_data.Provider, owner *Account) (map[commonpb.AccountType][]*AccountRecords, error) {
	res := make(map[commonpb.AccountType][]*AccountRecords)

	infoRecordsByType, err := data.GetLatestAccountInfosByOwnerAddress(ctx, owner.publicKey.ToBase58())
	if err != account.ErrAccountInfoNotFound && err != nil {
		return nil, err
	}

	if len(infoRecordsByType) == 0 {
		return res, nil
	}

	var timelockAccounts []string
	for _, infoRecords := range infoRecordsByType {
		for _, infoRecord := range infoRecords {
			if infoRecord.IsTimelock() {
				timelockAccounts = append(timelockAccounts, infoRecord.TokenAccount)
			}
		}
	}

	timelockRecordsByVault, err := data.GetTimelockByVaultBatch(ctx, timelockAccounts...)
	if err != nil {
		return nil, err
	}

	for _, generalRecords := range infoRecordsByType {
		for _, generalRecord := range generalRecords {
			var timelockRecord *timelock.Record
			var ok bool
			if generalRecord.IsTimelock() {
				timelockRecord, ok = timelockRecordsByVault[generalRecord.TokenAccount]
				if !ok {
					return nil, errors.New("timelock record unexpectedly doesn't exist")
				}
			}

			res[generalRecord.AccountType] = append(res[generalRecord.AccountType], &AccountRecords{
				General:  generalRecord,
				Timelock: timelockRecord,
			})
		}
	}

	// The record should never exist, but this is precautionary. Pre-privacy timelock
	// accounts should only be used in a migration.
	delete(res, commonpb.AccountType_LEGACY_PRIMARY_2022)

	return res, nil
}

// GetLatestCodeTimelockAccountRecordsForOwner is a utility wrapper over GetLatestTokenAccountRecordsForOwner
// that filters for Code Timelock accounts.
func GetLatestCodeTimelockAccountRecordsForOwner(ctx context.Context, data code_data.Provider, owner *Account) (map[commonpb.AccountType][]*AccountRecords, error) {
	res := make(map[commonpb.AccountType][]*AccountRecords)

	recordsByType, err := GetLatestTokenAccountRecordsForOwner(ctx, data, owner)
	if err != nil {
		return nil, err
	}

	for _, recordsList := range recordsByType {
		for _, records := range recordsList {
			if records.IsTimelock() {
				res[records.General.AccountType] = append(res[records.General.AccountType], records)
			}
		}
	}

	return res, nil
}

// GetLegacyPrimary2022AccountRecordsIfNotMigrated gets a faked AccountRecords
// for the LEGACY_PRIMARY_2022 account associated with the provided owner. If
// the account doesn't exist, or was migrated, then ErrNoPrivacyMigration2022
// is returned.
//
// Note: Legacy Timelock accounts were always Kin accounts
//
// todo: Needs tests here, but most already exist in account service
func GetLegacyPrimary2022AccountRecordsIfNotMigrated(ctx context.Context, data code_data.Provider, owner *Account) (*AccountRecords, error) {
	tokenAccount, err := owner.ToTimelockVault(timelock_token_v1.DataVersionLegacy, KinMintAccount)
	if err != nil {
		return nil, err
	}

	timelockRecord, err := data.GetTimelockByVault(ctx, tokenAccount.PublicKey().ToBase58())
	if err == timelock.ErrTimelockNotFound {
		return nil, ErrNoPrivacyMigration2022
	} else if err != nil {
		return nil, err
	}

	// Timelock account is closed, so it doesn't need migrating
	if timelockRecord.IsClosed() {
		return nil, ErrNoPrivacyMigration2022
	}

	// Client has already submitted an intent to migrate to privacy
	_, err = data.GetLatestIntentByInitiatorAndType(ctx, intent.MigrateToPrivacy2022, owner.PublicKey().ToBase58())
	if err == nil {
		return nil, ErrNoPrivacyMigration2022
	} else if err != intent.ErrIntentNotFound && err != nil {
		return nil, err
	}

	// Fake an account info record, since we don't save it for legacy primary 2022
	// accounts, but require it for downstream functions.
	accountInfoRecord := &account.Record{
		OwnerAccount:     owner.PublicKey().ToBase58(),
		AuthorityAccount: owner.PublicKey().ToBase58(),
		TokenAccount:     timelockRecord.VaultAddress,
		MintAccount:      KinMintAccount.PublicKey().ToBase58(),
		AccountType:      commonpb.AccountType_LEGACY_PRIMARY_2022,
		Index:            0,
	}

	return &AccountRecords{
		General:  accountInfoRecord,
		Timelock: timelockRecord,
	}, nil
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
