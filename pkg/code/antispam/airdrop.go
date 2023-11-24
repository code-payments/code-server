package antispam

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

var (
	deviceCheckV2ReleaseDate = time.Date(2023, time.October, 26, 0, 0, 0, 0, time.UTC)
)

// AllowWelcomeBonus determines whether a phone-verified owner account can receive
// the welcome bonus. The objective here is to limit attacks against our airdropper's
// Kin balance.
//
// todo: This needs tests
func (g *Guard) AllowWelcomeBonus(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowWelcomeBonus")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method": "AllowWelcomeBonus",
		"owner":  owner.PublicKey().ToBase58(),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionWelcomeBonus, "ip banned")
		return false, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionWelcomeBonus, "not phone verified")
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	if g.isSuspiciousWelcomeBonus(ctx, verification.PhoneNumber) {
		log.Info("denying suspicious welcome bonus")
		recordDenialEvent(ctx, actionWelcomeBonus, "suspicious welcome bonus")
		return false, nil
	}

	// Deny abusers from known phone ranges
	if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
		log.Info("denying phone prefix")
		recordDenialEvent(ctx, actionWelcomeBonus, "phone prefix banned")
		return false, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionWelcomeBonus, "user banned")
			return false, nil
		}

		// Staff users have unlimited access to enable testing and demoing.
		if user.IsStaffUser {
			return true, nil
		}
	case identity.ErrNotFound:
	default:
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting user identity by phone view")
		return false, err
	}

	phoneEvent, err := g.data.GetLatestPhoneEventForNumberByType(ctx, verification.PhoneNumber, phone.EventTypeVerificationCodeSent)
	switch err {
	case nil:
		// Deny from regions where we're currently under attack
		if phoneEvent.PhoneMetadata.MobileCountryCode != nil {
			if _, ok := g.conf.restrictedMobileCountryCodes[*phoneEvent.PhoneMetadata.MobileCountryCode]; ok {
				log.WithField("region", *phoneEvent.PhoneMetadata.MobileCountryCode).Info("region is restricted")
				recordDenialEvent(ctx, actionWelcomeBonus, "region restricted")
				return false, nil
			}
		}

		// Deny from mobile networks where we're currently under attack
		if phoneEvent.PhoneMetadata.MobileNetworkCode != nil {
			if _, ok := g.conf.restrictedMobileNetworkCodes[*phoneEvent.PhoneMetadata.MobileNetworkCode]; ok {
				log.WithField("mobile_network", *phoneEvent.PhoneMetadata.MobileNetworkCode).Info("mobile network is restricted")
				recordDenialEvent(ctx, actionWelcomeBonus, "mobile network restricted")
				return false, nil
			}
		}
	case phone.ErrEventNotFound:
	default:
		log.WithError(err).Warn("failure getting phone event")
		return false, err
	}

	return true, nil
}

// Special rules based on observed behaviour
func (g *Guard) isSuspiciousWelcomeBonus(ctx context.Context, phoneNumber string) bool {
	return false
}

// AllowReferralBonus determines whether a phone-verified owner account can receive
// a referral bonus. The objective here is to limit attacks against our airdropper's
// Kin balance.
//
// todo: This needs tests
func (g *Guard) AllowReferralBonus(
	ctx context.Context,
	referrerOwner,
	onboardedOwner,
	airdropperOwner *common.Account,
	quarksGivenByReferrer uint64,
	exchangedIn currency.Code,
	nativeAmount float64,
) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowReferralBonus")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":          "AllowReferralBonus",
		"referrer_owner":  referrerOwner.PublicKey().ToBase58(),
		"onboarded_owner": onboardedOwner.PublicKey().ToBase58(),
		"exchanged_in":    exchangedIn,
		"native_amount":   nativeAmount,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	if quarksGivenByReferrer < g.conf.minReferralAmount {
		log.Info("insufficient quarks given by referrer")
		recordDenialEvent(ctx, actionReferralBonus, "insufficient quarks given by referrer")
		return false, nil
	}

	for _, owner := range []*common.Account{referrerOwner, onboardedOwner} {
		verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
		if err == phone.ErrVerificationNotFound {
			// Owner account was never phone verified, so deny the action.
			log.Info("owner account is not phone verified")
			recordDenialEvent(ctx, actionReferralBonus, "not phone verified")
			return false, nil
		} else if err != nil {
			tracer.OnError(err)
			log.WithError(err).Warn("failure getting phone verification record")
			return false, err
		}

		log := log.WithField("phone", verification.PhoneNumber)

		// Deny abusers from known phone ranges
		if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
			log.Info("denying phone prefix")
			recordDenialEvent(ctx, actionReferralBonus, "phone prefix banned")
			return false, nil
		}

		if owner.PublicKey().ToBase58() == referrerOwner.PublicKey().ToBase58() && isSusipiciousReferral(verification.PhoneNumber, exchangedIn, nativeAmount) {
			log.Info("denying suspicious referral")
			recordDenialEvent(ctx, actionReferralBonus, "suspicious referral denied")
			return false, nil
		}

		user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
		switch err {
		case nil:
			// Deny banned users forever
			if user.IsBanned {
				log.Info("denying banned user")
				recordDenialEvent(ctx, actionReferralBonus, "user banned")
				return false, nil
			}

			// Staff users have unlimited access to enable testing and demoing.
			if user.IsStaffUser {
				return true, nil
			}

			if owner.PublicKey().ToBase58() == onboardedOwner.PublicKey().ToBase58() && user.CreatedAt.Before(deviceCheckV2ReleaseDate) {
				log.Info("denying pre-device check v2 onboarded user")
				recordDenialEvent(ctx, actionReferralBonus, "pre-device check v2 onboarded user")
				return false, nil
			}
		case identity.ErrNotFound:
		default:
			tracer.OnError(err)
			log.WithError(err).Warn("failure getting user identity by phone view")
			return false, err
		}

		phoneEvent, err := g.data.GetLatestPhoneEventForNumberByType(ctx, verification.PhoneNumber, phone.EventTypeVerificationCodeSent)
		switch err {
		case nil:
			// Deny from regions where we're currently under attack
			if phoneEvent.PhoneMetadata.MobileCountryCode != nil {
				if _, ok := g.conf.restrictedMobileCountryCodes[*phoneEvent.PhoneMetadata.MobileCountryCode]; ok {
					log.WithField("region", *phoneEvent.PhoneMetadata.MobileCountryCode).Info("region is restricted")
					recordDenialEvent(ctx, actionReferralBonus, "region restricted")
					return false, nil
				}

			}

			// Deny from mobile networks where we're currently under attack
			if phoneEvent.PhoneMetadata.MobileNetworkCode != nil {
				if _, ok := g.conf.restrictedMobileNetworkCodes[*phoneEvent.PhoneMetadata.MobileNetworkCode]; ok {
					log.WithField("mobile_network", *phoneEvent.PhoneMetadata.MobileNetworkCode).Info("mobile network is restricted")
					recordDenialEvent(ctx, actionReferralBonus, "mobile network restricted")
					return false, nil
				}
			}
		case phone.ErrEventNotFound:
		default:
			log.WithError(err).Warn("failure getting phone event")
			return false, err
		}
	}

	count, err := g.data.GetIntentCountWithOwnerInteractionsForAntispam(
		ctx,
		airdropperOwner.PublicKey().ToBase58(),
		referrerOwner.PublicKey().ToBase58(),
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.maxReferralsPerDay {
		log.Info("phone is rate limited by daily referral bonus count")
		recordDenialEvent(ctx, actionReferralBonus, "daily limit exceeded")
		return false, nil
	}

	return true, nil
}

// Special rules based on observed behaviour
func isSusipiciousReferral(phoneNumber string, exchangedIn currency.Code, nativeAmount float64) bool {
	return false
}
