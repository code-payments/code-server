package pg

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
)

func CheckNoRows(inErr, outErr error) error {
	if errors.Is(inErr, sql.ErrNoRows) {
		return outErr
	}
	return inErr
}

func CheckUniqueViolation(inErr, outErr error) error {
	if inErr != nil {
		var pgErr *pgconn.PgError
		if errors.As(inErr, &pgErr) {
			if pgErr.Code == pgerrcode.UniqueViolation {
				return outErr
			}
		}
	}
	return inErr
}

func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgerrcode.UniqueViolation {
			return true
		}
	}

	return false
}
