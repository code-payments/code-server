package antispam

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowOpenAccounts determines whether a phone-verified owner account can create
// a Code account via an open accounts intent. The objective here is to limit attacks
// against our Subsidizer's SOL balance.
func (g *Guard) AllowOpenAccounts(ctx context.Context, owner *common.Account, deviceToken *string) (bool, func() error, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowOpenAccounts")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method": "AllowOpenAccounts",
		"owner":  owner.PublicKey().ToBase58(),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionOpenAccounts, "ip banned")
		return false, nil, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionOpenAccounts, "not phone verified")
		return false, nil, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, nil, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	// Deny abusers from known phone ranges
	if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
		log.Info("denying phone prefix")
		recordDenialEvent(ctx, actionOpenAccounts, "phone prefix banned")
		return false, nil, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionOpenAccounts, "user banned")
			return false, nil, nil
		}

		// Staff users have unlimited access to enable testing and demoing.
		if user.IsStaffUser {
			return true, func() error { return nil }, nil
		}
	case identity.ErrNotFound:
	default:
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting user identity by phone view")
		return false, nil, err
	}

	// Account creation limit since the beginning of time
	count, err := g.data.GetIntentCountForAntispam(
		ctx,
		intent.OpenAccounts,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Unix(0, 0),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, nil, err
	}

	// Device-based restrictions guaranteeing 1 free account per valid device

	_, isAndroidDev := g.conf.androidDevsByPhoneNumber[verification.PhoneNumber]
	if !isAndroidDev {
		if deviceToken == nil {
			log.Info("denying attempt without device token")
			recordDenialEvent(ctx, actionOpenAccounts, "device token missing")
			return false, nil, nil
		}

		isValidDeviceToken, reason, err := g.deviceVerifier.IsValid(ctx, *deviceToken)
		if err != nil {
			log.WithError(err).Warn("failure performing device validation check")
			return false, nil, err
		} else if !isValidDeviceToken {
			log.WithField("reason", reason).Info("denying fake device")
			recordDenialEvent(ctx, actionOpenAccounts, "fake device")
			return false, nil, nil
		}

		hasCreatedFreeAccount, err := g.deviceVerifier.HasCreatedFreeAccount(ctx, *deviceToken)
		if err != nil {
			log.WithError(err).Warn("failure performing free account check for device")
			return false, nil, err
		} else if hasCreatedFreeAccount {
			log.Info("denying duplicate device")
			recordDenialEvent(ctx, actionOpenAccounts, "duplicate device")
			return false, nil, nil
		}
	}

	// Note: If we have multiple accounts per phone number, then this affects the
	// incenvtives logic, which uses this assumption.
	if int(count) >= 1 {
		log.Info("phone is rate limited by lifetime account creation count")
		recordDenialEvent(ctx, actionOpenAccounts, "lifetime limit exceeded")
		return false, nil, nil
	}

	onFreeAccountCreated := func() error {
		if isAndroidDev {
			return nil
		}
		return g.deviceVerifier.MarkCreatedFreeAccount(ctx, *deviceToken)
	}
	return true, onFreeAccountCreated, nil
}

// AllowSendPayment determines whether a phone-verified owner account is allowed to
// make a send public/private payment intent. The objective is to limit pressure on
// the scheduling layer.
func (g *Guard) AllowSendPayment(ctx context.Context, owner *common.Account, isPublic bool, destination *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSendPayment")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method": "AllowSendPayment",
		"owner":  owner.PublicKey().ToBase58(),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionSendPayment, "ip banned")
		return false, nil
	}

	if isPublic && isExternalAddressBanned(destination) {
		log.WithField("address", destination.PublicKey().ToBase58()).Info("external address is banned")
		recordDenialEvent(ctx, actionSendPayment, "external address banned")
		return false, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionSendPayment, "not phone verified")
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	// Deny abusers from known phone ranges
	if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
		log.Info("denying phone prefix")
		recordDenialEvent(ctx, actionSendPayment, "phone prefix banned")
		return false, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionSendPayment, "user banned")
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

	// Time-based rate limit per phone number across all sends
	rateLimited, err := g.limiter.denyPaymentByPhone(verification.PhoneNumber)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure checking phone rate limit")
		return false, err
	} else if rateLimited {
		log.Info("phone is rate limited by time")
		recordDenialEvent(ctx, actionSendPayment, "rate limit exceeded")
		return false, nil
	}

	// Count limits are applied separately for public and private payments. Each
	// has their own costs.
	intentType := intent.SendPrivatePayment
	if isPublic {
		intentType = intent.SendPublicPayment
	}

	// Five minute payment limit by phone number
	count, err := g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-5*time.Minute),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerFiveMinutes {
		log.Info("phone is rate limited by five minute payment count")
		recordDenialEvent(ctx, actionSendPayment, "five minute limit exceeded")
		return false, nil
	}

	// Hourly payment limit by phone number
	count, err = g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-1*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerHour {
		log.Info("phone is rate limited by hourly payment count")
		recordDenialEvent(ctx, actionSendPayment, "hourly limit exceeded")
		return false, nil
	}

	// Daily payment limit by phone number
	count, err = g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerDay {
		log.Info("phone is rate limited by daily payment count")
		recordDenialEvent(ctx, actionSendPayment, "daily limit exceeded")
		return false, nil
	}

	return true, nil
}

// AllowReceivePayments determines whether a phone-verified owner account is allowed to
// make a public/private receive payments intent. The objective is to limit pressure on
// the scheduling layer.
func (g *Guard) AllowReceivePayments(ctx context.Context, owner *common.Account, isPublic bool) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowReceivePayments")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method": "AllowReceivePayments",
		"owner":  owner.PublicKey().ToBase58(),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionReceivePayments, "ip banned")
		return false, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionReceivePayments, "not phone verified")
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	// Deny abusers from known phone ranges
	if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
		log.Info("denying phone prefix")
		recordDenialEvent(ctx, actionReceivePayments, "phone prefix banned")
		return false, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionReceivePayments, "user banned")
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

	// Time-based rate limit per phone number
	rateLimited, err := g.limiter.denyPaymentByPhone(verification.PhoneNumber)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure checking phone rate limit")
		return false, err
	} else if rateLimited {
		log.Info("phone is rate limited by time")
		recordDenialEvent(ctx, actionReceivePayments, "rate limit exceeded")
		return false, nil
	}

	// Count limits are applied separately for public and private payments. Each
	// has their own costs.
	intentType := intent.ReceivePaymentsPrivately
	if isPublic {
		intentType = intent.ReceivePaymentsPublicly
	}

	// Five minute payment limit by phone number
	count, err := g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-5*time.Minute),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerFiveMinutes {
		log.Info("phone is rate limited by five minute payment count")
		recordDenialEvent(ctx, actionReceivePayments, "five minute limit exceeded")
		return false, nil
	}

	// Hourly payment limit by phone number
	count, err = g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-1*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerHour {
		log.Info("phone is rate limited by hourly payment count")
		recordDenialEvent(ctx, actionReceivePayments, "hourly limit exceeded")
		return false, nil
	}

	// Daily payment limit by phone number
	count, err = g.data.GetIntentCountForAntispam(
		ctx,
		intentType,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.paymentsPerDay {
		log.Info("phone is rate limited by daily payment count")
		recordDenialEvent(ctx, actionReceivePayments, "daily limit exceeded")
		return false, nil
	}

	return true, nil
}

// AllowEstablishNewRelationship determines whether a phone-verified owner account is allowed
// to establish a new relationship
func (g *Guard) AllowEstablishNewRelationship(ctx context.Context, owner *common.Account, relationshipTo string) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowEstablishNewRelationship")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method":          "AllowEstablishNewRelationship",
		"owner":           owner.PublicKey().ToBase58(),
		"relationship_to": relationshipTo,
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionEstablishNewRelationship, "ip banned")
		return false, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionEstablishNewRelationship, "not phone verified")
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	// Deny abusers from known phone ranges
	if hasBannedPhoneNumberPrefix(verification.PhoneNumber) {
		log.Info("denying phone prefix")
		recordDenialEvent(ctx, actionEstablishNewRelationship, "phone prefix banned")
		return false, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionEstablishNewRelationship, "user banned")
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

	// Basic rate limit across the last day
	count, err := g.data.GetIntentCountForAntispam(
		ctx,
		intent.EstablishRelationship,
		verification.PhoneNumber,
		[]intent.State{intent.StateUnknown, intent.StatePending, intent.StateFailed, intent.StateConfirmed},
		time.Now().Add(-24*time.Hour),
	)
	if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting intent count")
		return false, err
	}

	if count >= g.conf.maxNewRelationshipsPerDay {
		log.Info("phone is rate limited by daily count")
		recordDenialEvent(ctx, actionEstablishNewRelationship, "daily limit exceeded")
		return false, nil
	}

	return true, nil
}
