package user

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/rate"

	grpc_client "github.com/code-payments/code-server/pkg/grpc/client"
)

// limiter limits user-based service calls by both IP and phone number
type limiter struct {
	log         *logrus.Entry
	ip          rate.Limiter
	phoneNumber rate.Limiter
}

// NewLimiter creates a new Limiter.
func newLimiter(ctor rate.LimiterCtor, phoneNumberLimit, ipLimit float64) *limiter {
	return &limiter{
		log:         logrus.StandardLogger().WithField("type", "user/limiter"),
		ip:          ctor(ipLimit),
		phoneNumber: ctor(phoneNumberLimit),
	}
}

func (l *limiter) allowPhoneLinking(ctx context.Context, phoneNumber string) bool {
	log := l.log.WithFields(logrus.Fields{
		"method": "allowPhoneLinking",
		"phone":  phoneNumber,
	})

	ip, err := grpc_client.GetIPAddr(ctx)
	if err != nil {
		log.WithError(err).Warn("failure getting client ip")
	} else {
		log = log.WithField("ip", ip)
		allow, err := l.allowIP(ip)
		if err != nil {
			log.WithError(err).Warn("failure checking ip rate limit")
		} else if !allow {
			log.Trace("ip is rate limited")
			return false
		}
	}

	allow, err := l.allowPhoneNumber(phoneNumber)
	if err != nil {
		log.WithError(err).Warn("failure checking phone number rate limit")
	} else if !allow {
		log.Trace("phone number is rate limited")
		return false
	}

	return true
}

func (l *limiter) allowIP(ipAddr string) (bool, error) {
	allowed, err := l.ip.Allow(ipAddr)
	if err != nil {
		return true, err
	}
	return allowed, nil
}

func (l *limiter) allowPhoneNumber(phoneNumber string) (bool, error) {
	allowed, err := l.phoneNumber.Allow(phoneNumber)
	if err != nil {
		return true, err
	}
	return allowed, nil
}
