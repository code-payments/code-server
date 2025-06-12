package pg

import (
	"database/sql"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/pkg/errors"
)

func CheckNoRows(inErr, outErr error) error {
	if IsNoRows(inErr) {
		return outErr
	}
	return inErr
}

func IsNoRows(err error) bool {
	if err == nil {
		return false
	}
	return err == sql.ErrNoRows
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
