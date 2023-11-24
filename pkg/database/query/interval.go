package query

import (
	"github.com/pkg/errors"
)

type Interval uint8

const (
	IntervalRaw Interval = iota
	IntervalHour
	IntervalDay
	IntervalWeek
	IntervalMonth
)

var AllIntervals = []Interval{
	IntervalRaw,
	IntervalHour,
	IntervalDay,
	IntervalWeek,
	IntervalMonth,
}

func ToInterval(val string) (Interval, error) {
	switch val {
	case "raw":
		return IntervalRaw, nil
	case "hour":
		return IntervalHour, nil
	case "day":
		return IntervalDay, nil
	case "week":
		return IntervalWeek, nil
	case "month":
		return IntervalMonth, nil
	default:
		return 0, errors.Errorf("unexpected value: %v", val)
	}
}

func FromInterval(val Interval) (string, error) {
	switch val {
	case IntervalRaw:
		return "raw", nil
	case IntervalHour:
		return "hour", nil
	case IntervalDay:
		return "day", nil
	case IntervalWeek:
		return "week", nil
	case IntervalMonth:
		return "month", nil
	default:
		return "", errors.Errorf("unexpected value: %v", val)
	}
}

func ToIntervalWithFallback(val string, fallback Interval) Interval {
	res, err := ToInterval(val)
	if err != nil {
		return fallback
	}
	return res
}

func FromIntervalWithFallback(val Interval, fallback string) string {
	res, err := FromInterval(val)
	if err != nil {
		return fallback
	}
	return res
}
