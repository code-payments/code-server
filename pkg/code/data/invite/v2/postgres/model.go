package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	pgutil "github.com/code-payments/code-server/pkg/database/postgres"
	"github.com/code-payments/code-server/pkg/phone"

	"github.com/code-payments/code-server/pkg/code/data/invite/v2"
)

const (
	userTableName           = "codewallet__core_inviteuserv2"
	influencerCodeTableName = "codewallet__core_influencercode"
	waitlistTableName       = "codewallet__core_invitewaitlist"
)

type userModel struct {
	Id                     sql.NullInt64  `db:"id"`
	PhoneNumber            string         `db:"phone_number"`
	InvitedBy              sql.NullString `db:"invited_by"`
	Invited                time.Time      `db:"invited"`
	InviteCount            int32          `db:"invite_count"`
	InvitesSent            int32          `db:"invites_sent"`
	DepositInvitesReceived bool           `db:"deposit_invites_received"`
	IsRevoked              bool           `db:"is_revoked"`
}

type influencerCodeModel struct {
	Code        string    `db:"code"`
	InviteCount int32     `db:"invite_count"`
	InvitesSent int32     `db:"invites_sent"`
	IsRevoked   bool      `db:"is_revoked"`
	ExpiresAt   time.Time `db:"expires_at"`
}

type waitlistEntryModel struct {
	Id          sql.NullInt64 `db:"id"`
	PhoneNumber string        `db:"phone_number"`
	IsVerified  bool          `db:"is_verified"`
	CreatedAt   time.Time     `db:"created_at"`
}

func toUserModel(obj *invite.User) (*userModel, error) {
	if err := obj.Validate(); err != nil {
		return nil, err
	}

	invitedByValue := sql.NullString{}
	if obj.InvitedBy != nil {
		invitedByValue.Valid = true
		invitedByValue.String = *obj.InvitedBy
	}

	return &userModel{
		PhoneNumber:            obj.PhoneNumber,
		InvitedBy:              invitedByValue,
		Invited:                obj.Invited,
		InviteCount:            int32(obj.InviteCount),
		InvitesSent:            int32(obj.InvitesSent),
		DepositInvitesReceived: obj.DepositInvitesReceived,
		IsRevoked:              obj.IsRevoked,
	}, nil
}

func fromUserModel(obj *userModel) *invite.User {
	var invitedByValue *string
	if obj.InvitedBy.Valid {
		invitedByValue = &obj.InvitedBy.String
	}

	return &invite.User{
		PhoneNumber:            obj.PhoneNumber,
		InvitedBy:              invitedByValue,
		Invited:                obj.Invited,
		InviteCount:            uint32(obj.InviteCount),
		InvitesSent:            uint32(obj.InvitesSent),
		DepositInvitesReceived: obj.DepositInvitesReceived,
		IsRevoked:              obj.IsRevoked,
	}
}

func (m *userModel) dbPut(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelDefault,
	})
	if err != nil {
		return err
	}

	// Check if the InvitedBy value is a phone number by

	if m.InvitedBy.Valid {
		var updateQuery string

		if phone.IsE164Format(m.InvitedBy.String) {
			updateQuery = `UPDATE ` + userTableName + `
				SET invites_sent = invites_sent + 1
				WHERE phone_number = $1 AND is_revoked = false AND invites_sent < invite_count
			`
		} else {
			updateQuery = `UPDATE ` + influencerCodeTableName + `
				SET invites_sent = invites_sent + 1
				WHERE code = $1 AND is_revoked = false AND invites_sent < invite_count AND expires_at > NOW()
			`
		}

		result, err := tx.ExecContext(ctx, updateQuery, m.InvitedBy.String)
		if err != nil {
			tx.Rollback()
			return err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			tx.Rollback()
			return err
		} else if rowsAffected == 0 {
			tx.Rollback()
			return invite.ErrInviteCountExceeded
		}
	}

	insertQuery := `INSERT INTO ` + userTableName + `
		(phone_number, invited_by, invited, invite_count, invites_sent, deposit_invites_received, is_revoked)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = tx.ExecContext(
		ctx,
		insertQuery,
		m.PhoneNumber,
		m.InvitedBy,
		m.Invited.UTC(),
		m.InviteCount,
		m.InvitesSent,
		m.DepositInvitesReceived,
		m.IsRevoked,
	)
	if err != nil {
		tx.Rollback()
		return pgutil.CheckUniqueViolation(err, invite.ErrAlreadyExists)
	}

	return tx.Commit()
}

func toWaitlistEntryModel(obj *invite.WaitlistEntry) *waitlistEntryModel {
	return &waitlistEntryModel{
		PhoneNumber: obj.PhoneNumber,
		IsVerified:  obj.IsVerified,
		CreatedAt:   obj.CreatedAt,
	}
}

func (m *waitlistEntryModel) dbSave(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + waitlistTableName + `
		(phone_number, is_verified, created_at)
		VALUES ($1, $2, $3)
		RETURNING id, phone_number, is_verified, created_at
	`

	err := db.QueryRowxContext(
		ctx,
		query,
		m.PhoneNumber,
		m.IsVerified,
		m.CreatedAt,
	).StructScan(m)
	return pgutil.CheckUniqueViolation(err, nil)
}

func dbGetByNumber(ctx context.Context, db *sqlx.DB, phoneNumber string) (*userModel, error) {
	res := &userModel{}

	query := `SELECT id, phone_number, invited_by, invited, invite_count, invites_sent, deposit_invites_received, is_revoked FROM ` + userTableName + `
		WHERE phone_number = $1`

	err := db.GetContext(ctx, res, query, phoneNumber)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, invite.ErrUserNotFound)
	}
	return res, nil
}

func dbFilterInvitedNumbers(ctx context.Context, db *sqlx.DB, phoneNumbers []string) ([]*userModel, error) {
	if len(phoneNumbers) == 0 {
		return nil, nil
	}

	phoneNumberArgs := make([]string, len(phoneNumbers))
	for i, phoneNumber := range phoneNumbers {
		phoneNumberArgs[i] = fmt.Sprintf("'%s'", phoneNumber)
	}

	// todo: is there a better way to construct the query?
	query := fmt.Sprintf(
		"SELECT DISTINCT id, phone_number, invited_by, invited, invite_count, invites_sent, is_revoked FROM %s WHERE phone_number IN (%s)",
		userTableName,
		strings.Join(phoneNumberArgs, ","),
	)

	var res []*userModel

	err := db.SelectContext(ctx, &res, query)
	return res, pgutil.CheckNoRows(err, nil)
}

func dbGiveInvitesForDeposit(ctx context.Context, db *sqlx.DB, phoneNumber string, amount int) error {
	if amount <= 0 {
		return errors.New("invalid invite count delta")
	}

	query := `UPDATE ` + userTableName + `
		SET invite_count = invite_count + $2, deposit_invites_received = true
		WHERE phone_number = $1 AND deposit_invites_received IS FALSE AND is_revoked = false`

	_, err := db.ExecContext(ctx, query, phoneNumber, amount)
	return pgutil.CheckNoRows(err, nil)
}

func toInfluencerCodeModel(obj *invite.InfluencerCode) (*influencerCodeModel, error) {
	return &influencerCodeModel{
		Code:        obj.Code,
		InviteCount: int32(obj.InviteCount),
		InvitesSent: int32(obj.InvitesSent),
		IsRevoked:   obj.IsRevoked,
		ExpiresAt:   obj.ExpiresAt,
	}, nil
}

func fromInfluencerCodeModel(obj *influencerCodeModel) *invite.InfluencerCode {
	return &invite.InfluencerCode{
		Code:        obj.Code,
		InviteCount: uint32(obj.InviteCount),
		InvitesSent: uint32(obj.InvitesSent),
		IsRevoked:   obj.IsRevoked,
		ExpiresAt:   obj.ExpiresAt,
	}
}

func (m *influencerCodeModel) dbPut(ctx context.Context, db *sqlx.DB) error {
	query := `INSERT INTO ` + influencerCodeTableName + `
		(code, invite_count, invites_sent, is_revoked, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (code) DO UPDATE SET invite_count = $2, invites_sent = $3, is_revoked = $4, expires_at = $5
	`

	_, err := db.ExecContext(
		ctx,
		query,
		m.Code,
		m.InviteCount,
		m.InvitesSent,
		m.IsRevoked,
		m.ExpiresAt,
	)
	return pgutil.CheckUniqueViolation(err, nil)
}

func (m *influencerCodeModel) dbClaim(ctx context.Context, db *sqlx.DB) error {
	query := `UPDATE ` + influencerCodeTableName + `
		SET invites_sent = invites_sent + 1
		WHERE code = $1 AND is_revoked = false AND invites_sent < invite_count AND expires_at > NOW()
		RETURNING code, invite_count, invites_sent, is_revoked, expires_at`

	err := db.QueryRowxContext(ctx, query, m.Code).StructScan(m)

	return pgutil.CheckNoRows(err, invite.ErrInfluencerCodeNotFound)
}

func dbGetInfluencerCode(ctx context.Context, db *sqlx.DB, code string) (*influencerCodeModel, error) {
	res := &influencerCodeModel{}

	query := `SELECT code, invite_count, invites_sent, is_revoked, expires_at FROM ` + influencerCodeTableName + `
		WHERE code = $1`

	err := db.GetContext(ctx, res, query, code)
	if err != nil {
		return nil, pgutil.CheckNoRows(err, invite.ErrInfluencerCodeNotFound)
	}

	return res, nil
}
