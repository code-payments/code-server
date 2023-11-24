package antispam

import (
	"github.com/code-payments/code-server/pkg/rate"
)

type limiter struct {
	paymentsByPhone rate.Limiter
}

func newLimiter(ctor rate.LimiterCtor, paymentsByPhoneLimit float64) *limiter {
	return &limiter{
		paymentsByPhone: ctor(paymentsByPhoneLimit),
	}
}

func (l *limiter) denyPaymentByPhone(phoneNumber string) (bool, error) {
	allowed, err := l.paymentsByPhone.Allow(phoneNumber)
	if err != nil {
		// Being extra safe in the event of failure. Do not allow anything.
		return true, err
	}
	return !allowed, nil
}
