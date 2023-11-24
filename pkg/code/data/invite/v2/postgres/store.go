package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
)

type store struct {
	db *sqlx.DB
}

// New returns a new postgres backed invite.v2.Store
func New(db *sql.DB) invite.Store {
	return &store{
		db: sqlx.NewDb(db, "pgx"),
	}
}

// GetUser implements invite.v2.Store.Getuser
func (s *store) GetUser(ctx context.Context, phoneNumber string) (*invite.User, error) {
	model, err := dbGetByNumber(ctx, s.db, phoneNumber)
	if err != nil {
		return nil, err
	}

	return fromUserModel(model), nil
}

// PutUser implements invite.v2.Store.PutUser
func (s *store) PutUser(ctx context.Context, user *invite.User) error {
	model, err := toUserModel(user)
	if err != nil {
		return err
	}

	return model.dbPut(ctx, s.db)
}

// GetInfluencerCode gets an influencer code.
func (s *store) GetInfluencerCode(ctx context.Context, code string) (*invite.InfluencerCode, error) {
	model, err := dbGetInfluencerCode(ctx, s.db, code)
	if err != nil {
		return nil, err
	}

	return fromInfluencerCodeModel(model), nil
}

// PutInfluencerCode stores an influencer code.
func (s *store) PutInfluencerCode(ctx context.Context, influencerCode *invite.InfluencerCode) error {
	model, err := toInfluencerCodeModel(influencerCode)
	if err != nil {
		return err
	}

	return model.dbPut(ctx, s.db)
}

// ClaimInfluencerCode claims an influencer code.
func (s *store) ClaimInfluencerCode(ctx context.Context, code string) error {
	model, err := dbGetInfluencerCode(ctx, s.db, code)
	if err != nil {
		return err
	}

	if model.IsRevoked {
		return invite.ErrInfluencerCodeRevoked
	}

	if model.InvitesSent >= model.InviteCount {
		return invite.ErrInviteCountExceeded
	}

	if (!model.ExpiresAt.IsZero()) && (model.ExpiresAt.Before(time.Now())) {
		return invite.ErrInfluencerCodeExpired
	}

	return model.dbClaim(ctx, s.db)
}

// FilterInvitedNumbers implements invite.v2.Store.FilterInvitedNumbers
func (s *store) FilterInvitedNumbers(ctx context.Context, phoneNumbers []string) ([]*invite.User, error) {
	models, err := dbFilterInvitedNumbers(ctx, s.db, phoneNumbers)
	if err != nil {
		return nil, err
	}

	users := make([]*invite.User, len(models))
	for i, model := range models {
		users[i] = fromUserModel(model)
	}
	return users, nil
}

// PutOnWaitlist implements invite.v2.Store.PutOnWaitlist
func (s *store) PutOnWaitlist(ctx context.Context, entry *invite.WaitlistEntry) error {
	model := toWaitlistEntryModel(entry)
	return model.dbSave(ctx, s.db)
}

// GiveInvitesForDeposit implements invite.v2.Store.GiveInvitesForDeposit
func (s *store) GiveInvitesForDeposit(ctx context.Context, phoneNumber string, amount int) error {
	return dbGiveInvitesForDeposit(ctx, s.db, phoneNumber, amount)
}
