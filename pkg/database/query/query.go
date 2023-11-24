package query

import (
	"errors"
	"time"
)

var (
	ErrQueryNotSupported = errors.New("the requested query option is not supported")
)

type SupportedOptions byte

const (
	CanLimitResults     SupportedOptions = 0x01
	CanSortBy           SupportedOptions = 0x01 << 1
	CanBucketBy         SupportedOptions = 0x01 << 2
	CanQueryByCursor    SupportedOptions = 0x01 << 3
	CanQueryByStartTime SupportedOptions = 0x01 << 4
	CanQueryByEndTime   SupportedOptions = 0x01 << 5
	CanFilterBy         SupportedOptions = 0x01 << 6
)

type QueryOptions struct {
	Supported SupportedOptions

	Start time.Time
	End   time.Time

	Interval Interval
	SortBy   Ordering
	Limit    uint64
	Cursor   Cursor
	FilterBy Filter
}

type Option func(*QueryOptions) error

func (qo *QueryOptions) check(cap SupportedOptions) bool {
	return qo.Supported&cap != cap
}

func (qo *QueryOptions) Apply(opts ...Option) error {
	for _, o := range opts {
		err := o(qo)
		if err != nil {
			return err
		}
	}
	return nil
}

func WithInterval(val Interval) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanBucketBy) {
			return ErrQueryNotSupported
		}
		qo.Interval = val
		return nil
	}
}

func WithFilter(val Filter) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanFilterBy) {
			return ErrQueryNotSupported
		}
		qo.FilterBy = val
		return nil
	}
}

func WithDirection(val Ordering) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanSortBy) {
			return ErrQueryNotSupported
		}
		qo.SortBy = val
		return nil
	}
}

func WithLimit(val uint64) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanLimitResults) {
			return ErrQueryNotSupported
		}
		qo.Limit = val
		return nil
	}
}

func WithCursor(val []byte) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanQueryByCursor) {
			return ErrQueryNotSupported
		}
		qo.Cursor = val
		return nil
	}
}

func WithStartTime(val time.Time) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanQueryByStartTime) {
			return ErrQueryNotSupported
		}
		qo.Start = val
		return nil
	}
}

func WithEndTime(val time.Time) Option {
	return func(qo *QueryOptions) error {
		if qo.check(CanQueryByEndTime) {
			return ErrQueryNotSupported
		}
		qo.End = val
		return nil
	}
}
