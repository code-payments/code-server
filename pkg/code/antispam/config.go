package antispam

import (
	"time"

	"github.com/code-payments/code-server/pkg/kin"
)

// todo: migrate this to the new way of doing configs

const (
	// todo: separate limits by public/private?
	defaultPaymentsPerDay         = 250
	defaultPaymentsPerHour        = 50
	defaultPaymentsPerFiveMinutes = 25
	defaultTimePerPayment         = time.Second / 4

	defaultMaxNewRelationshipsPerDay = 50

	defaultMinReferralAmount  = 100 * kin.QuarksPerKin
	defaultMaxReferralsPerDay = 10
)

type conf struct {
	paymentsPerDay         uint64
	paymentsPerHour        uint64
	paymentsPerFiveMinutes uint64
	timePerPayment         time.Duration

	maxNewRelationshipsPerDay uint64

	minReferralAmount  uint64
	maxReferralsPerDay uint64

	restrictedMobileCountryCodes map[int]struct{}
	restrictedMobileNetworkCodes map[int]struct{}

	androidDevsByPhoneNumber map[string]struct{}
}

// Option configures a Guard with an overrided configuration value
type Option func(c *conf)

// WithDailyPaymentLimit overrides the default daily payment limit. The value
// specifies the maximum number of payments that can initiated per phone number
// per day.
func WithDailyPaymentLimit(limit uint64) Option {
	return func(c *conf) {
		c.paymentsPerDay = limit
	}
}

// WithHourlyPaymentLimit overrides the default hourly payment limit. The value
// specifies the maximum number of payments that can initiated per phone number
// per hour.
func WithHourlyPaymentLimit(limit uint64) Option {
	return func(c *conf) {
		c.paymentsPerHour = limit
	}
}

// WithFiveMinutePaymentLimit overrides the default five minute payment limit.
// The value specifies the maximum number of payments that can initiated per
// phone number per five minutes.
func WithFiveMinutePaymentLimit(limit uint64) Option {
	return func(c *conf) {
		c.paymentsPerFiveMinutes = limit
	}
}

// WithPaymentRateLimit overrides the default payment rate limit. The value
// specifies the minimum time between payments per phone number.
func WithPaymentRateLimit(d time.Duration) Option {
	return func(c *conf) {
		c.timePerPayment = d
	}
}

// WithMaxNewRelationshipsPerDay overrides the default maximum number of new relationships
// a phone number can create per day.
func WithMaxNewRelationshipsPerDay(limit uint64) Option {
	return func(c *conf) {
		c.maxNewRelationshipsPerDay = limit
	}
}

// WithMinReferralAmount overrides the default minimum referral amount. The value specifies
// the minimum amount that must be given to a new user to consider a referral bonus.
func WithMinReferralAmount(amount uint64) Option {
	return func(c *conf) {
		c.minReferralAmount = amount
	}
}

// WithMaxReferralsPerDay overrides the default maximum referrals per day. The value specifies
// the maximum number of times a user can be given a referral bonus in a day.
func WithMaxReferralsPerDay(limit uint64) Option {
	return func(c *conf) {
		c.maxReferralsPerDay = limit
	}
}

// WithRestrictedMobileCountryCodes overrides the default set of restricted mobile country
// codes. The values specify the mobile country codes with restricted access to prevent
// spam waves from problematic regions.
func WithRestrictedMobileCountryCodes(mccs ...int) Option {
	return func(c *conf) {
		c.restrictedMobileCountryCodes = make(map[int]struct{})
		for _, mcc := range mccs {
			c.restrictedMobileCountryCodes[mcc] = struct{}{}
		}
	}
}

// WithAndroidDevs configures a set of open source Android devs that get to bypass certain
// antispam measures to enable testing. Android is currently behind the latest antispam
// system requirements, and will fail things like device attestation.
func WithAndroidDevs(phoneNumbers ...string) Option {
	return func(c *conf) {
		c.androidDevsByPhoneNumber = make(map[string]struct{})
		for _, phoneNumber := range phoneNumbers {
			c.androidDevsByPhoneNumber[phoneNumber] = struct{}{}
		}
	}
}

// WithRestrictedMobileNetworkCodes overrides the default set of restricted mobile network
// codes. The values specify the mobile network codes with restricted access to prevent
// attacks from fraudulent operators.
//
// todo: Need to be careful with these. MNC may not be unique to a MCC. The
// ones provided here don't exist anywhere.
func WithRestrictedMobileNetworkCodes(mncs ...int) Option {
	return func(c *conf) {
		c.restrictedMobileNetworkCodes = make(map[int]struct{})
		for _, mnc := range mncs {
			c.restrictedMobileNetworkCodes[mnc] = struct{}{}
		}
	}
}

func applyOptions(opts ...Option) *conf {
	defaultConfig := &conf{
		paymentsPerDay:         defaultPaymentsPerDay,
		paymentsPerHour:        defaultPaymentsPerHour,
		paymentsPerFiveMinutes: defaultPaymentsPerFiveMinutes,
		timePerPayment:         defaultTimePerPayment,

		maxNewRelationshipsPerDay: defaultMaxNewRelationshipsPerDay,

		minReferralAmount:  defaultMinReferralAmount,
		maxReferralsPerDay: defaultMaxReferralsPerDay,

		restrictedMobileCountryCodes: make(map[int]struct{}),
		restrictedMobileNetworkCodes: make(map[int]struct{}),

		androidDevsByPhoneNumber: make(map[string]struct{}),
	}

	for _, opt := range opts {
		opt(defaultConfig)
	}

	return defaultConfig
}
