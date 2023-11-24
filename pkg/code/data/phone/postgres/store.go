package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/phone"
)

type store struct {
	db *sqlx.DB
}

// New returns a postgres backed phone.Store.
func New(db *sql.DB) phone.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// SaveVerification implements phone.Store.SaveVerification
func (s *store) SaveVerification(ctx context.Context, v *phone.Verification) error {
	model, err := toVerificationModel(v)
	if err != nil {
		return err
	}

	return model.dbSave(ctx, s.db)
}

// GetVerification implements phone.Store.GetVerification
func (s *store) GetVerification(ctx context.Context, account, phoneNumber string) (*phone.Verification, error) {
	model, err := dbGetVerification(ctx, s.db, account, phoneNumber)
	if err != nil {
		return nil, err
	}

	return fromVerificationModel(model), nil
}

// GetLatestVerificationForAccount implements phone.Store.GetLatestVerificationForAccount
func (s *store) GetLatestVerificationForAccount(ctx context.Context, account string) (*phone.Verification, error) {
	model, err := dbGetLatestVerificationForAccount(ctx, s.db, account)
	if err != nil {
		return nil, err
	}

	return fromVerificationModel(model), nil
}

// GetLatestVerificationForNumber implements phone.Store.GetLatestVerificationForNumber
func (s *store) GetLatestVerificationForNumber(ctx context.Context, phoneNumber string) (*phone.Verification, error) {
	model, err := dbGetLatestVerificationForNumber(ctx, s.db, phoneNumber)
	if err != nil {
		return nil, err
	}

	return fromVerificationModel(model), nil
}

// GetAllVerificationsForNumber implements phone.Store.GetAllVerificationsForNumber
func (s *store) GetAllVerificationsForNumber(ctx context.Context, phoneNumber string) ([]*phone.Verification, error) {
	models, err := dbGetAllVerificationsForNumber(ctx, s.db, phoneNumber)
	if err != nil {
		return nil, err
	}

	verifications := make([]*phone.Verification, len(models))
	for i, model := range models {
		verifications[i] = fromVerificationModel(model)
	}

	return verifications, nil
}

// SaveLinkingToken implements phone.Store.SaveLinkingToken
func (s *store) SaveLinkingToken(ctx context.Context, token *phone.LinkingToken) error {
	model, err := toLinkingTokenModel(token)
	if err != nil {
		return err
	}

	return model.dbSave(ctx, s.db)
}

// UseLinkingToken implements phone.Store.UseLinkingToken
func (s *store) UseLinkingToken(ctx context.Context, phoneNumber, code string) error {
	return dbUseLinkingToken(ctx, s.db, phoneNumber, code)
}

// FilterVerifiedNumbers implements phone.Store.FilterVerifiedNumbers
func (s *store) FilterVerifiedNumbers(ctx context.Context, phoneNumbers []string) ([]string, error) {
	return dbFilterVerifiedNumbers(ctx, s.db, phoneNumbers)
}

// GetSettings implements phone.Store.GetSettings
func (s *store) GetSettings(ctx context.Context, phoneNumber string) (*phone.Settings, error) {
	models, err := dbGetOwnerAccountSettings(ctx, s.db, phoneNumber)
	if err != nil {
		return nil, err
	}

	settings := &phone.Settings{
		PhoneNumber:    phoneNumber,
		ByOwnerAccount: make(map[string]*phone.OwnerAccountSetting),
	}

	for _, model := range models {
		settings.ByOwnerAccount[model.OwnerAccount] = fromOwnerAccountSettingModel(model)
	}

	return settings, nil
}

// SaveOwnerAccountSetting implements phone.Store.SaveOwnerAccountSetting
func (s *store) SaveOwnerAccountSetting(ctx context.Context, phoneNumber string, newSettings *phone.OwnerAccountSetting) error {
	model, err := toOwnerAccountSettingModel(phoneNumber, newSettings)
	if err != nil {
		return err
	}

	return model.dbSave(ctx, s.db)
}

// PutEvent implements phone.Store.PutEvent
func (s *store) PutEvent(ctx context.Context, event *phone.Event) error {
	model, err := toEventModel(event)
	if err != nil {
		return err
	}

	return model.dbSave(ctx, s.db)
}

// GetLatestEventForNumberByType implements phone.Store.GetLatestEventForNumberByType
func (s *store) GetLatestEventForNumberByType(ctx context.Context, phoneNumber string, eventType phone.EventType) (*phone.Event, error) {
	model, err := dbGetLatestEventForNumberByType(ctx, s.db, phoneNumber, eventType)
	if err != nil {
		return nil, err
	}

	return fromEventModel(model), nil
}

// CountEventsForVerificationByType implements phone.Store.CountEventsForVerificationByType
func (s *store) CountEventsForVerificationByType(ctx context.Context, verification string, eventType phone.EventType) (uint64, error) {
	return dbCountEventsForVerificationByType(ctx, s.db, verification, eventType)
}

// CountEventsForNumberByTypeSinceTimestamp implements phone.Store.CountEventsForNumberByTypeSinceTimestamp
func (s *store) CountEventsForNumberByTypeSinceTimestamp(ctx context.Context, phoneNumber string, eventType phone.EventType, since time.Time) (uint64, error) {
	return dbCountEventsForNumberByTypeSinceTimestamp(ctx, s.db, phoneNumber, eventType, since)
}

// CountUniqueVerificationIdsForNumberSinceTimestamp implements phone.Store.CountUniqueVerificationIdsForNumberSinceTimestamp
func (s *store) CountUniqueVerificationIdsForNumberSinceTimestamp(ctx context.Context, phoneNumber string, since time.Time) (uint64, error) {
	return dbCountUniqueVerificationIdsForNumberSinceTimestamp(ctx, s.db, phoneNumber, since)
}
