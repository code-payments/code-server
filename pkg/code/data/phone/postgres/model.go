package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	phoneutil "github.com/code-payments/code-server/pkg/phone"

	"github.com/code-payments/code-server/pkg/code/data/phone"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
)

const (
	verificationTableName = "codewallet__core_phoneverification"
	linkingTokenTableName = "codewallet__core_phonelinkingtoken"
	settingTableName      = "codewallet__core_phonesetting"
	eventTableName        = "codewallet__core_phoneevent"
)

type verificationModel struct {
	Id             sql.NullInt64 `db:"id"`
	PhoneNumber    string        `db:"phone_number"`
	OwnerAccount   string        `db:"owner_account"`
	CreatedAt      time.Time     `db:"created_at"`
	LastVerifiedAt time.Time     `db:"last_verified_at"`
}

func toVerificationModel(obj *phone.Verification) (*verificationModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &verificationModel{
		PhoneNumber:    obj.PhoneNumber,
		OwnerAccount:   obj.OwnerAccount,
		CreatedAt:      obj.CreatedAt,
		LastVerifiedAt: obj.LastVerifiedAt,
	}, nil
}

func fromVerificationModel(obj *verificationModel) *phone.Verification {
	return &phone.Verification{
		PhoneNumber:    obj.PhoneNumber,
		OwnerAccount:   obj.OwnerAccount,
		CreatedAt:      obj.CreatedAt,
		LastVerifiedAt: obj.LastVerifiedAt,
	}
}

func (m *verificationModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + verificationTableName + `
		(
			phone_number, owner_account, created_at, last_verified_at
		)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (phone_number, owner_account)
		DO UPDATE
			SET last_verified_at = $4
			WHERE ` + verificationTableName + `.phone_number = $1 AND ` + verificationTableName + `.owner_account = $2 AND ` + verificationTableName + `.last_verified_at < $4
		RETURNING id, phone_number, owner_account, last_verified_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.PhoneNumber,
		m.OwnerAccount,
		m.CreatedAt.UTC(),
		m.LastVerifiedAt.UTC(),
	).StructScan(m)
	return pgutil.CheckNoRows(err, phone.ErrInvalidVerification)
}

func dbGetVerification(ctx context.Context, db *sqlx.DB, account, phoneNumber string) (*verificationModel, error) {
	res := &verificationModel{}

	query := `SELECT id, phone_number, owner_account, created_at, last_verified_at FROM ` + verificationTableName + `
		WHERE owner_account = $1 AND phone_number = $2
	`

	err := db.GetContext(ctx, res, query, account, phoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, phone.ErrVerificationNotFound)
	}
	return res, nil
}

func dbGetLatestVerificationForAccount(ctx context.Context, db *sqlx.DB, account string) (*verificationModel, error) {
	res := &verificationModel{}

	query := `SELECT id, phone_number, owner_account, created_at, last_verified_at FROM ` + verificationTableName + `
		WHERE owner_account = $1
		ORDER BY last_verified_at DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, account)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, phone.ErrVerificationNotFound)
	}
	return res, nil
}

func dbGetLatestVerificationForNumber(ctx context.Context, db *sqlx.DB, phoneNumber string) (*verificationModel, error) {
	res := &verificationModel{}

	query := `SELECT id, phone_number, owner_account, created_at, last_verified_at FROM ` + verificationTableName + `
		WHERE phone_number = $1
		ORDER BY last_verified_at DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, phoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, phone.ErrVerificationNotFound)
	}
	return res, nil
}

func dbGetAllVerificationsForNumber(ctx context.Context, db *sqlx.DB, phoneNumber string) ([]*verificationModel, error) {
	var res []*verificationModel

	query := `SELECT id, phone_number, owner_account, created_at, last_verified_at FROM ` + verificationTableName + `
		WHERE phone_number = $1
		ORDER BY last_verified_at DESC
	`

	err := db.SelectContext(ctx, &res, query, phoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, phone.ErrVerificationNotFound)
	}
	if len(res) == 0 {
		return nil, phone.ErrVerificationNotFound
	}
	return res, nil
}

type linkingTokenModel struct {
	Id                sql.NullInt64 `db:"id"`
	PhoneNumber       string        `db:"phone_number"`
	Code              string        `db:"code"`
	CurrentCheckCount uint32        `db:"current_check_count"`
	MaxCheckCount     uint32        `db:"max_check_count"`
	ExpiresAt         time.Time     `db:"expires_at"`
}

func toLinkingTokenModel(obj *phone.LinkingToken) (*linkingTokenModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	return &linkingTokenModel{
		PhoneNumber:       obj.PhoneNumber,
		Code:              obj.Code,
		CurrentCheckCount: obj.CurrentCheckCount,
		MaxCheckCount:     obj.MaxCheckCount,
		ExpiresAt:         obj.ExpiresAt,
	}, nil
}

func fromLinkingTokenModel(obj *linkingTokenModel) *phone.LinkingToken {
	return &phone.LinkingToken{
		PhoneNumber:       obj.PhoneNumber,
		Code:              obj.Code,
		CurrentCheckCount: uint32(obj.CurrentCheckCount),
		MaxCheckCount:     uint32(obj.MaxCheckCount),
		ExpiresAt:         obj.ExpiresAt,
	}
}

func (m *linkingTokenModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + linkingTokenTableName + `
		(
			phone_number, code, current_check_count, max_check_count, expires_at
		)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (phone_number)
		DO UPDATE
			SET code = $2, current_check_count = $3, max_check_count = $4, expires_at = $5
			WHERE ` + linkingTokenTableName + `.phone_number = $1
		RETURNING id, phone_number, code, current_check_count, max_check_count, expires_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.PhoneNumber,
		m.Code,
		m.CurrentCheckCount,
		m.MaxCheckCount,
		m.ExpiresAt.UTC(),
	).StructScan(m)
	return err
}

func dbUseLinkingToken(ctx context.Context, db *sqlx.DB, phoneNumber, code string) error {
	res := &linkingTokenModel{}

	query := `UPDATE ` + linkingTokenTableName + `
		SET current_check_count = current_check_count + 1
		WHERE phone_number = $1
		RETURNING id, phone_number, code, current_check_count, max_check_count, expires_at`

	err := db.QueryRowxContext(
		ctx,
		query,
		phoneNumber,
	).StructScan(res)
	if err != nil {
		return pgutil.CheckNoRows(err, phone.ErrLinkingTokenNotFound)
	}

	query = `DELETE FROM ` + linkingTokenTableName + `
		WHERE phone_number = $1 AND code = $2
		RETURNING id, phone_number, code, current_check_count, max_check_count, expires_at`

	err = db.QueryRowxContext(
		ctx,
		query,
		phoneNumber,
		code,
	).StructScan(res)

	if err != nil {
		return pgutil.CheckNoRows(err, phone.ErrLinkingTokenNotFound)
	}

	if res.ExpiresAt.Before(time.Now()) || res.CurrentCheckCount > res.MaxCheckCount {
		return phone.ErrLinkingTokenNotFound
	}

	return nil
}

func dbFilterVerifiedNumbers(ctx context.Context, db *sqlx.DB, phoneNumbers []string) ([]string, error) {
	if len(phoneNumbers) == 0 {
		return nil, nil
	}

	phoneNumberArgs := make([]string, len(phoneNumbers))
	for i, phoneNumber := range phoneNumbers {
		phoneNumberArgs[i] = fmt.Sprintf("'%s'", phoneNumber)
	}

	// todo: is there a better way to construct the query?
	query := fmt.Sprintf(
		"SELECT DISTINCT phone_number FROM %s WHERE phone_number IN (%s) ORDER BY phone_number ASC",
		verificationTableName,
		strings.Join(phoneNumberArgs, ","),
	)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}

	var result []string

	for rows.Next() {
		var phoneNumber string
		err := rows.Scan(&phoneNumber)
		if err != nil {
			return nil, err
		}

		result = append(result, phoneNumber)
	}

	return result, nil
}

type ownerAccountSettingModel struct {
	Id            sql.NullInt64 `db:"id"`
	PhoneNumber   string        `db:"phone_number"`
	OwnerAccount  string        `db:"owner_account"`
	IsUnlinked    sql.NullBool  `db:"is_unlinked"`
	CreatedAt     time.Time     `db:"created_at"`
	LastUpdatedAt time.Time     `db:"last_updated_at"`
}

func toOwnerAccountSettingModel(phoneNumber string, obj *phone.OwnerAccountSetting) (*ownerAccountSettingModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	if !phoneutil.IsE164Format(phoneNumber) {
		return nil, errors.New("phone number doesn't match E.164 standard")
	}

	var isUnlinked sql.NullBool
	if obj.IsUnlinked != nil {
		isUnlinked.Valid = true
		isUnlinked.Bool = *obj.IsUnlinked
	}

	return &ownerAccountSettingModel{
		PhoneNumber:   phoneNumber,
		OwnerAccount:  obj.OwnerAccount,
		IsUnlinked:    isUnlinked,
		CreatedAt:     obj.CreatedAt,
		LastUpdatedAt: obj.LastUpdatedAt,
	}, nil
}

func fromOwnerAccountSettingModel(obj *ownerAccountSettingModel) *phone.OwnerAccountSetting {
	var isUnlinked *bool
	if obj.IsUnlinked.Valid {
		isUnlinked = &obj.IsUnlinked.Bool
	}

	return &phone.OwnerAccountSetting{
		OwnerAccount:  obj.OwnerAccount,
		IsUnlinked:    isUnlinked,
		CreatedAt:     obj.CreatedAt,
		LastUpdatedAt: obj.LastUpdatedAt,
	}
}

func (m *ownerAccountSettingModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + settingTableName + `
		(
			phone_number, owner_account, is_unlinked, created_at, last_updated_at
		)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (phone_number, owner_account)
		DO UPDATE
			SET 
			is_unlinked = COALESCE($3, ` + settingTableName + `.is_unlinked),
				last_updated_at = $5
			WHERE ` + settingTableName + `.phone_number = $1 AND ` + settingTableName + `.owner_account = $2
		RETURNING id, phone_number, owner_account, is_unlinked, created_at, last_updated_at`

	return db.QueryRowxContext(
		ctx,
		query,
		m.PhoneNumber,
		m.OwnerAccount,
		m.IsUnlinked,
		m.CreatedAt.UTC(),
		m.LastUpdatedAt.UTC(),
	).StructScan(m)
}

func dbGetOwnerAccountSettings(ctx context.Context, db *sqlx.DB, phoneNumber string) ([]*ownerAccountSettingModel, error) {
	var res []*ownerAccountSettingModel

	query := `SELECT id, phone_number, owner_account, is_unlinked, created_at, last_updated_at FROM ` + settingTableName + `
		WHERE phone_number = $1`

	err := db.SelectContext(ctx, &res, query, phoneNumber)

	return res, pgutil.CheckNoRows(err, nil)
}

type eventModel struct {
	Id                sql.NullInt64 `db:"id"`
	EventType         int           `db:"event_type"`
	VerificationId    string        `db:"verification_id"`
	PhoneNumber       string        `db:"phone_number"`
	PhoneType         sql.NullInt64 `db:"phone_type"`
	MobileCountryCode sql.NullInt64 `db:"mobile_country_code"`
	MobileNetworkCode sql.NullInt64 `db:"mobile_network_code"`
	CreatedAt         time.Time     `db:"created_at"`
}

func toEventModel(obj *phone.Event) (*eventModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	var phoneType sql.NullInt64
	if obj.PhoneMetadata.Type != nil {
		phoneType.Valid = true
		phoneType.Int64 = int64(*obj.PhoneMetadata.Type)
	}

	var mobileCountryCode sql.NullInt64
	if obj.PhoneMetadata.MobileCountryCode != nil {
		mobileCountryCode.Valid = true
		mobileCountryCode.Int64 = int64(*obj.PhoneMetadata.MobileCountryCode)
	}

	var mobileNetworkCode sql.NullInt64
	if obj.PhoneMetadata.MobileNetworkCode != nil {
		mobileNetworkCode.Valid = true
		mobileNetworkCode.Int64 = int64(*obj.PhoneMetadata.MobileNetworkCode)
	}

	return &eventModel{
		EventType:         int(obj.Type),
		VerificationId:    obj.VerificationId,
		PhoneNumber:       obj.PhoneNumber,
		PhoneType:         phoneType,
		MobileCountryCode: mobileCountryCode,
		MobileNetworkCode: mobileNetworkCode,
		CreatedAt:         obj.CreatedAt,
	}, nil
}

func fromEventModel(obj *eventModel) *phone.Event {
	var phoneType *phoneutil.Type
	if obj.PhoneType.Valid {
		value := phoneutil.Type(obj.PhoneType.Int64)
		phoneType = &value
	}

	var mobileCountryCode *int
	if obj.MobileCountryCode.Valid {
		value := int(obj.MobileCountryCode.Int64)
		mobileCountryCode = &value
	}

	var mobileNetworkCode *int
	if obj.MobileNetworkCode.Valid {
		value := int(obj.MobileNetworkCode.Int64)
		mobileNetworkCode = &value
	}

	return &phone.Event{
		Type:           phone.EventType(obj.EventType),
		VerificationId: obj.VerificationId,
		PhoneNumber:    obj.PhoneNumber,
		PhoneMetadata: &phoneutil.Metadata{
			PhoneNumber:       obj.PhoneNumber,
			Type:              phoneType,
			MobileCountryCode: mobileCountryCode,
			MobileNetworkCode: mobileNetworkCode,
		},
		CreatedAt: obj.CreatedAt,
	}
}

func (m *eventModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + eventTableName + `
		(
			event_type, verification_id, phone_number, phone_type, mobile_country_code, mobile_network_code, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, event_type, verification_id, phone_number, phone_type, mobile_country_code, mobile_network_code, created_at`

	return db.QueryRowxContext(
		ctx,
		query,
		m.EventType,
		m.VerificationId,
		m.PhoneNumber,
		m.PhoneType,
		m.MobileCountryCode,
		m.MobileNetworkCode,
		m.CreatedAt,
	).StructScan(m)
}

func dbGetLatestEventForNumberByType(ctx context.Context, db *sqlx.DB, phoneNumber string, eventType phone.EventType) (*eventModel, error) {
	res := &eventModel{}

	query := `SELECT id, event_type, verification_id, phone_number, phone_type, mobile_country_code, mobile_network_code, created_at FROM ` + eventTableName + `
		WHERE phone_number = $1 and event_type = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := db.GetContext(ctx, res, query, phoneNumber, eventType)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, phone.ErrEventNotFound)
	}
	return res, nil
}

func dbCountEventsForVerificationByType(ctx context.Context, db *sqlx.DB, verification string, eventType phone.EventType) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + eventTableName + ` WHERE verification_id = $1 AND event_type = $2`
	err := db.GetContext(ctx, &res, query, verification, eventType)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbCountEventsForNumberByTypeSinceTimestamp(ctx context.Context, db *sqlx.DB, phoneNumber string, eventType phone.EventType, since time.Time) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(*) FROM ` + eventTableName + ` WHERE phone_number = $1 AND event_type = $2 AND created_at >= $3`
	err := db.GetContext(ctx, &res, query, phoneNumber, eventType, since)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func dbCountUniqueVerificationIdsForNumberSinceTimestamp(ctx context.Context, db *sqlx.DB, phoneNumber string, since time.Time) (uint64, error) {
	var res uint64

	query := `SELECT COUNT(DISTINCT verification_id) FROM ` + eventTableName + ` WHERE phone_number = $1 AND created_at >= $2`
	err := db.GetContext(ctx, &res, query, phoneNumber, since)
	if err != nil {
		return 0, err
	}

	return res, nil
}
