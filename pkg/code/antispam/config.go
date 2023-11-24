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

	defaultPhoneVerificationInterval       = 10 * time.Minute
	defaultPhoneVerificationsPerInterval   = 5
	defaultTimePerSmsVerificationCodeSend  = 30 * time.Second
	defaultTimePerSmsVerificationCodeCheck = time.Second

	defaultMaxNewRelationshipsPerDay = 50

	defaultMinReferralAmount  = 100 * kin.QuarksPerKin
	defaultMaxReferralsPerDay = 10
)

type conf struct {
	paymentsPerDay         uint64
	paymentsPerHour        uint64
	paymentsPerFiveMinutes uint64
	timePerPayment         time.Duration

	phoneVerificationInterval       time.Duration
	phoneVerificationsPerInternval  uint64
	timePerSmsVerificationCodeSend  time.Duration
	timePerSmsVerificationCodeCheck time.Duration

	maxNewRelationshipsPerDay uint64

	minReferralAmount  uint64
	maxReferralsPerDay uint64

	restrictedMobileCountryCodes map[int]struct{}
	restrictedMobileNetworkCodes map[int]struct{}
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

// WithPhoneVerificationInterval overrides the default phone verification interval.
// The value specifies the time window at which unique verifications are evaluated
// per phone number.
func WithPhoneVerificationInterval(d time.Duration) Option {
	return func(c *conf) {
		c.phoneVerificationInterval = d
	}
}

// WithPhoneVerificationsPerInterval overrides the default number of phone verifications
// in an interval. The value specifies the number of unique phone verifications that can
// happen within the configurable time window per phone number.
func WithPhoneVerificationsPerInterval(limit uint64) Option {
	return func(c *conf) {
		c.phoneVerificationsPerInternval = limit
	}
}

// WithTimePerSmsVerificationCodeSend overrides the default time per SMS verifications codes
// sent. The value specifies the minimum time that must be waited to send consecutive SMS
// verification codes per phone number.
func WithTimePerSmsVerificationCodeSend(d time.Duration) Option {
	return func(c *conf) {
		c.timePerSmsVerificationCodeSend = d
	}
}

// WithTimePerSmsVerificationCheck overrides the default time per SMS verifications codes
// checked. The value specifies the minimum time that must be waited to consecutively check
// SMS verification codes per phone number.
func WithTimePerSmsVerificationCheck(d time.Duration) Option {
	return func(c *conf) {
		c.timePerSmsVerificationCodeCheck = d
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

		phoneVerificationInterval:       defaultPhoneVerificationInterval,
		phoneVerificationsPerInternval:  defaultPhoneVerificationsPerInterval,
		timePerSmsVerificationCodeSend:  defaultTimePerSmsVerificationCodeSend,
		timePerSmsVerificationCodeCheck: defaultTimePerSmsVerificationCodeCheck,

		maxNewRelationshipsPerDay: defaultMaxNewRelationshipsPerDay,

		minReferralAmount:  defaultMinReferralAmount,
		maxReferralsPerDay: defaultMaxReferralsPerDay,

		restrictedMobileCountryCodes: make(map[int]struct{}),
		restrictedMobileNetworkCodes: make(map[int]struct{}),
	}

	for _, opt := range opts {
		opt(defaultConfig)
	}

	return defaultConfig
}
