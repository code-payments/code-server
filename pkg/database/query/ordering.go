package query

import (
	"github.com/pkg/errors"
)

// The ordering of a returned set of records
type Ordering uint

const (
	Ascending Ordering = iota
	Descending
)

func ToOrdering(val string) (Ordering, error) {
	switch val {
	case "asc":
		return Ascending, nil
	case "desc":
		return Descending, nil
	default:
		return 0, errors.Errorf("unexpected value: %v", val)
	}
}

func FromOrdering(val Ordering) (string, error) {
	switch val {
	case Ascending:
		return "asc", nil
	case Descending:
		return "desc", nil
	default:
		return "", errors.Errorf("unexpected value: %v", val)
	}
}

func ToOrderingWithFallback(val string, fallback Ordering) Ordering {
	res, err := ToOrdering(val)
	if err != nil {
		return fallback
	}
	return res
}

func FromOrderingWithFallback(val Ordering, fallback string) string {
	res, err := FromOrdering(val)
	if err != nil {
		return fallback
	}
	return res
}
