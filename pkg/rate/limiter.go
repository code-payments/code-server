package rate

import (
	"sync"

	"golang.org/x/time/rate"
)

// Limiter limits operations based on a provided key.
type Limiter interface {
	Allow(key string) (bool, error)
}

// LimiterCtor allows the creation of a Limiter using a provided rate.
type LimiterCtor func(rate float64) Limiter

type localRateLimiter struct {
	limit rate.Limit

	sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewRedisRateLimiter returns an in memory limiter.
func NewLocalRateLimiter(limit rate.Limit) Limiter {
	return &localRateLimiter{
		limit:    limit,
		limiters: make(map[string]*rate.Limiter),
	}
}

// Allow implements limiter.Allow.
func (l *localRateLimiter) Allow(key string) (bool, error) {
	l.Lock()
	limiter, ok := l.limiters[key]
	if !ok {
		limiter = rate.NewLimiter(l.limit, int(l.limit))
		l.limiters[key] = limiter
	}
	l.Unlock()

	return limiter.Allow(), nil
}

// NoLimiter never limits operations
type NoLimiter struct {
}

// Allow implements limiter.Allow.
func (n *NoLimiter) Allow(key string) (bool, error) {
	return true, nil
}
