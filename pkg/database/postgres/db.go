package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jmoiron/sqlx"
)

type txStructContextKey struct{}
type txIsolationContextKey struct{}

var (
	ErrAlreadyInTx = errors.New("already executing in existing db tx")
	ErrNotInTx     = errors.New("not executing in existing db tx")
)

// ExecuteRetryable Retry functions that perform non-transactional database operations.
func ExecuteRetryable(fn func() error) error {
	if err := fn(); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.SerializationFailure {
			// A recognised error type that can be retried.
			return ExecuteRetryable(fn)
		}
		return err
	}
	return nil
}

// ExecuteTxWithinCtx executes a DB transaction that's scoped to a call to fn. The transaction
// is passed along with the context. Once fn is complete, commit/rollback is called based
// on whether an error is returned.
func ExecuteTxWithinCtx(ctx context.Context, db *sqlx.DB, isolation sql.IsolationLevel, fn func(context.Context) error) error {
	if isolation == sql.LevelDefault {
		isolation = sql.LevelReadCommitted // Postgres default
	}

	existing := ctx.Value(txStructContextKey{})
	if existing != nil {
		return ErrAlreadyInTx
	}

	tx, err := db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: isolation,
	})
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, txStructContextKey{}, tx)
	ctx = context.WithValue(ctx, txIsolationContextKey{}, isolation)

	err = fn(ctx)
	if err != nil {
		// We always need to execute a Rollback() so sql.DB releases the connection.
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to rollback transaction: %w", rollbackErr)
		}

		return err
	}
	return tx.Commit()
}

// ExecuteInTx is meant for DB store implementations to execute an operation within
// the scope of a DB transaction. This method is aware of ExecuteTxWithinCtx, and
// will dynamically decide when to use a new or existing transaction, as well as
// where the respnosibilty for commit/rollback calls lie.
func ExecuteInTx(ctx context.Context, db *sqlx.DB, isolation sql.IsolationLevel, fn func(tx *sqlx.Tx) error) (err error) {
	if isolation == sql.LevelDefault {
		isolation = sql.LevelReadCommitted // Postgres default
	}

	tx, err := getTxFromCtx(ctx, isolation)
	if err != nil && err != ErrNotInTx {
		return err
	}

	var startedNewTx bool // To determine who is responsible for commit/rollback
	if err == ErrNotInTx {
		startedNewTx = true
		tx, err = db.BeginTxx(ctx, &sql.TxOptions{
			Isolation: isolation,
		})
		if err != nil {
			return err
		}
	}

	err = fn(tx)
	if err != nil {
		if startedNewTx {
			// We always need to execute a Rollback() so sql.DB releases the connection.
			if rollBackErr := tx.Rollback(); rollBackErr != nil {
				return fmt.Errorf("failed to rollback transaction: %w", rollBackErr)
			}
		}
		return err
	}
	if startedNewTx {
		return tx.Commit()
	}
	return nil
}

func getTxFromCtx(ctx context.Context, desiredIsolation sql.IsolationLevel) (*sqlx.Tx, error) {
	txFromCtx := ctx.Value(txStructContextKey{})
	if txFromCtx == nil {
		return nil, ErrNotInTx
	}

	isolationFromCtx := ctx.Value(txIsolationContextKey{})
	if isolationFromCtx == nil {
		return nil, errors.New("unexpectedly don't have isolation level set")
	}

	tx, ok := txFromCtx.(*sqlx.Tx)
	if !ok {
		return nil, errors.New("invalid type for tx")
	}

	currentIsolation, ok := isolationFromCtx.(sql.IsolationLevel)
	if !ok {
		return nil, errors.New("invalid type for isolation")
	}

	if currentIsolation < desiredIsolation {
		return nil, errors.New("current tx doesn't meet isolation level requirements")
	}

	return tx, nil
}
