package antispam

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowNewPhoneVerification determines whether a phone is allowed to start a
// new verification flow.
func (g *Guard) AllowNewPhoneVerification(ctx context.Context, phoneNumber string, deviceToken *string) (bool, Reason, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowNewPhoneVerification")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":       "AllowNewPhoneVerification",
		"phone_number": phoneNumber,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionNewPhoneVerification, "ip banned")
		return false, ReasonUnspecified, nil
	}

	// Deny users from sanctioned countries
	if isSanctionedPhoneNumber(phoneNumber) {
		log.Info("denying sanctioned country")
		recordDenialEvent(ctx, actionNewPhoneVerification, "sanctioned country")
		return false, ReasonUnsupportedCountry, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, phoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionNewPhoneVerification, "user banned")
			return false, ReasonUnspecified, nil
		}

		// Staff users have unlimited access to enable testing and demoing.
		if user.IsStaffUser {
			return true, ReasonUnspecified, nil
		}
	case identity.ErrNotFound:
	default:
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting user identity by phone view")
		return false, ReasonUnspecified, err
	}

	_, isAndroidDev := g.conf.androidDevsByPhoneNumber[phoneNumber]
	if !isAndroidDev {
		// Importantly, after staff checks so we don't gate testing on devices where
		// device tokens don't exist (eg. iOS simulator)
		if deviceToken == nil {
			log.Info("denying attempt without device token")
			recordDenialEvent(ctx, actionNewPhoneVerification, "device token missing")
			return false, ReasonUnsupportedDevice, nil
		}
		isValidDeviceToken, reason, err := g.deviceVerifier.IsValid(ctx, *deviceToken)
		if err != nil {
			log.WithError(err).Warn("failure performing device check")
			return false, ReasonUnspecified, err
		} else if !isValidDeviceToken {
			log.WithField("reason", reason).Info("denying fake device")
			recordDenialEvent(ctx, actionNewPhoneVerification, "fake device")
			return false, ReasonUnsupportedDevice, nil
		}
	}

	since := time.Now().Add(-1 * g.conf.phoneVerificationInterval)
	count, err := g.data.GetUniquePhoneVerificationIdCountForNumberSinceTimestamp(ctx, phoneNumber, since)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure counting unique verification ids for number")
		return false, ReasonUnspecified, err
	}

	if count >= g.conf.phoneVerificationsPerInternval {
		log.Info("phone is rate limited")
		recordDenialEvent(ctx, actionNewPhoneVerification, "rate limit exceeded")
		return false, ReasonUnspecified, nil
	}
	return true, ReasonUnspecified, nil
}

// AllowSendSmsVerificationCode determines whether a phone number can be sent
// a verification code over SMS.
func (g *Guard) AllowSendSmsVerificationCode(ctx context.Context, phoneNumber string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSendSmsVerificationCode")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":       "AllowSendSmsVerificationCode",
		"phone_number": phoneNumber,
	})

	since := time.Now().Add(-1 * g.conf.timePerSmsVerificationCodeSend)
	count, err := g.data.GetPhoneEventCountForNumberByTypeSinceTimestamp(ctx, phoneNumber, phone.EventTypeVerificationCodeSent, since)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure counting phone events")
		return false, err
	}

	if count > 0 {
		log.Info("phone is rate limited")
		recordDenialEvent(ctx, actionSendSmsVerificationCode, "rate limit exceeded")
		return false, nil
	}
	return true, nil
}

// AllowCheckSmsVerificationCode determines whether a phone number is allowed
// to check a SMS verification code.
func (g *Guard) AllowCheckSmsVerificationCode(ctx context.Context, phoneNumber string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowCheckSmsVerificationCode")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":       "AllowCheckSmsVerificationCode",
		"phone_number": phoneNumber,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	since := time.Now().Add(-1 * g.conf.timePerSmsVerificationCodeCheck)
	count, err := g.data.GetPhoneEventCountForNumberByTypeSinceTimestamp(ctx, phoneNumber, phone.EventTypeCheckVerificationCode, since)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure counting phone events")
		return false, err
	}

	if count > 0 {
		log.Info("phone is rate limited")
		recordDenialEvent(ctx, actionCheckSmsVerificationCode, "rate limit exceeded")
		return false, nil
	}
	return true, nil
}

// AllowLinkAccount determines whether an identity is allowed to link to an
// account.
//
// todo: this needs tests
func (g *Guard) AllowLinkAccount(ctx context.Context, ownerAccount *common.Account, phoneNumber string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowLinkAccount")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":        "AllowLinkAccount",
		"owner_account": ownerAccount.PublicKey().ToBase58(),
		"phone_number":  phoneNumber,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	previousPhoneVerificationRecord, err := g.data.GetLatestPhoneVerificationForAccount(ctx, ownerAccount.PublicKey().ToBase58())
	if err == nil {
		log = log.WithField("previous_phone_number", previousPhoneVerificationRecord.PhoneNumber)

		// Don't allow new links when the previous one is to a banned user.
		previousUserRecord, err := g.data.GetUserByPhoneView(ctx, previousPhoneVerificationRecord.PhoneNumber)
		if err == nil && previousUserRecord.IsBanned {
			log.Info("denying relink where previous user is banned")
			recordDenialEvent(ctx, actionLinkAccount, "previous user banned")
			return false, nil
		}
	} else if err != phone.ErrVerificationNotFound {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting previous phone verification record")
		return false, err
	}

	return true, nil
}
