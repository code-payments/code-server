package query

import (
	"fmt"
	"strconv"
)

const (
	defaultPagingLimit = 1024
)

// PaginateQuery returns a paginated query string for the given input options.
//
// The input query string is expected as follows:
//
//	"SELECT ... WHERE (...)" <- these brackets are not optional
//
// The output query string would be as follows:
//
//	"SELECT ... WHERE (...) AND id > ? ORDER BY ? LIMIT ?"
//	-or-
//	"SELECT ... WHERE (...) AND id < ? ORDER BY ? LIMIT ?"
//
// Example:
//
//		query := "SELECT * FROM table WHERE (state = $1 OR age > $2)"
//
//		opts := []interface{}{ state: 123, age: 45, }
//		cursor := 123
//		limit := 10
//		direction := Ascending
//
//	 PaginateQuery(query, opts, cursor, limit, direction)
//	 > "SELECT * FROM table WHERE (state = $1 OR age > $2) AND cursor > $3 ORDER BY id ASC LIMIT 10"
func PaginateQuery(
	query string, opts []interface{},
	cursor Cursor, limit uint64, direction Ordering,
) (string, []interface{}) {
	return PaginateQueryOnField(query, opts, cursor, limit, direction, "id")
}

func PaginateQueryOnField(
	query string, opts []interface{},
	cursor Cursor, limit uint64, direction Ordering, fieldName string,
) (string, []interface{}) {

	if len(cursor) > 0 {
		v := strconv.Itoa(len(opts) + 1)

		if direction == Ascending {
			query += fmt.Sprintf(" AND %s > $", fieldName) + v
		} else {
			query += fmt.Sprintf(" AND %s < $", fieldName) + v
		}

		opts = append(opts, cursor.ToUint64())
	}

	if direction == Ascending {
		query += fmt.Sprintf(" ORDER BY %s ASC", fieldName)
	} else {
		query += fmt.Sprintf(" ORDER BY %s DESC", fieldName)
	}

	if limit > 0 {
		v := strconv.Itoa(len(opts) + 1)

		query += " LIMIT $" + v

		opts = append(opts, limit)
	}

	return query, opts
}

func DefaultPaginationHandler(opts ...Option) (*QueryOptions, error) {
	req := QueryOptions{
		Limit:     defaultPagingLimit,
		SortBy:    Ascending,
		Supported: CanLimitResults | CanSortBy | CanQueryByCursor,
	}
	req.Apply(opts...)

	if req.Limit > defaultPagingLimit {
		return nil, ErrQueryNotSupported
	}

	return &req, nil
}

func DefaultPaginationHandlerWithLimit(limit uint64, opts ...Option) (*QueryOptions, error) {
	req := QueryOptions{
		Limit:     limit,
		SortBy:    Ascending,
		Supported: CanLimitResults | CanSortBy | CanQueryByCursor,
	}
	req.Apply(opts...)

	if req.Limit > limit {
		return nil, ErrQueryNotSupported
	}

	return &req, nil
}
