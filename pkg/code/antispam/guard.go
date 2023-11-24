package antispam

import (
	"github.com/oschwald/maxminddb-golang"
	"github.com/sirupsen/logrus"
	xrate "golang.org/x/time/rate"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/device"
	"github.com/code-payments/code-server/pkg/rate"
)

// todo: Generally, this package has evolved quickly, which means testing and
//       code structure isn't the most ideal. A refactor is needed at some point.
//       For example, there's a lot of shared commonalities between the various
//       Allow methods that are copied and not tested (phone prefix, ip, banned
//       users, etc.).

// Guard is an antispam guard that checks whether operations of interest are
// allowed to be performed.
//
// Note: Implementation assumes distributed locking has already occurred for
// all methods.
type Guard struct {
	log            *logrus.Entry
	data           code_data.Provider
	deviceVerifier device.Verifier
	maxmind        *maxminddb.Reader
	limiter        *limiter
	conf           *conf
}

func NewGuard(
	data code_data.Provider,
	deviceVerifier device.Verifier,
	maxmind *maxminddb.Reader,
	opts ...Option,
) *Guard {
	conf := applyOptions(opts...)

	// todo: need a global rate limiter
	limiter := newLimiter(func(r float64) rate.Limiter {
		return rate.NewLocalRateLimiter(xrate.Limit(r))
	}, float64(xrate.Every(conf.timePerPayment)))

	return &Guard{
		log:            logrus.StandardLogger().WithField("type", "antispam/guard"),
		data:           data,
		deviceVerifier: deviceVerifier,
		maxmind:        maxmind,
		limiter:        limiter,
		conf:           conf,
	}
}
