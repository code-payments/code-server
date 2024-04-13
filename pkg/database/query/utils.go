package query

import "strconv"

const (
	defaultPagingLimit = 1000
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
func PaginateQuery(query string, opts []interface{},
	cursor Cursor, limit uint64, direction Ordering) (string, []interface{}) {

	if len(cursor) > 0 {
		v := strconv.Itoa(len(opts) + 1)

		if direction == Ascending {
			query += " AND id > $" + v
		} else {
			query += " AND id < $" + v
		}

		opts = append(opts, cursor.ToUint64())
	}

	if direction == Ascending {
		query += " ORDER BY id ASC"
	} else {
		query += " ORDER BY id DESC"
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
	if err := req.Apply(opts...); err != nil {
		return nil, ErrQueryNotSupported
	}

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
	if err := req.Apply(opts...); err != nil {
		return nil, ErrQueryNotSupported
	}

	if req.Limit > limit {
		return nil, ErrQueryNotSupported
	}

	return &req, nil
}
