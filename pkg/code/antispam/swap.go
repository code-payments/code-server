package antispam

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/phone"
	"github.com/code-payments/code-server/pkg/code/data/user/identity"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/metrics"
)

// AllowSwap determines whether a phone-verified owner account can perform a swap.
// The objective here is to limit attacks against our Swap Subsidizer's SOL balance.
//
// todo: needs tests
func (g *Guard) AllowSwap(ctx context.Context, owner *common.Account) (bool, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "AllowSwap")
	defer tracer.End()

	log := g.log.WithFields(logrus.Fields{
		"method": "AllowSwap",
		"owner":  owner.PublicKey().ToBase58(),
	})
	log = client.InjectLoggingMetadata(ctx, log)

	// Deny abusers from known IPs
	if isIpBanned(ctx) {
		log.Info("ip is banned")
		recordDenialEvent(ctx, actionSwap, "ip banned")
		return false, nil
	}

	verification, err := g.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err == phone.ErrVerificationNotFound {
		// Owner account was never phone verified, so deny the action.
		log.Info("owner account is not phone verified")
		recordDenialEvent(ctx, actionSwap, "not phone verified")
		return false, nil
	} else if err != nil {
		tracer.OnError(err)
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}

	log = log.WithField("phone", verification.PhoneNumber)

	// Deny users from sanctioned countries
	if isSanctionedPhoneNumber(verification.PhoneNumber) {
		log.Info("denying sanctioned country")
		recordDenialEvent(ctx, actionSwap, "sanctioned country")
		return false, nil
	}

	user, err := g.data.GetUserByPhoneView(ctx, verification.PhoneNumber)
	switch err {
	case nil:
		// Deny banned users forever
		if user.IsBanned {
			log.Info("denying banned user")
			recordDenialEvent(ctx, actionSwap, "user banned")
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

	return true, nil
}
