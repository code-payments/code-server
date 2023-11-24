package antispam

import (
	"context"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	metricsStructName = "antispam.guard"

	eventName = "AntispamGuardDenial"

	actionOpenAccounts             = "OpenAccounts"
	actionSendPayment              = "SendPayment"
	actionReceivePayments          = "ReceivePayments"
	actionEstablishNewRelationship = "EstablishNewRelationship"

	actionNewPhoneVerification     = "NewPhoneVerification"
	actionSendSmsVerificationCode  = "SendSmsVerificationCode"
	actionCheckSmsVerificationCode = "CheckSmsVerificationCode"
	actionLinkAccount              = "LinkAccount"

	actionWelcomeBonus  = "WelcomeBonus"
	actionReferralBonus = "ReferralBonus"
)

func recordDenialEvent(ctx context.Context, action, reason string) {
	kvPairs := map[string]interface{}{
		"action": action,
		"reason": reason,
		"count":  1,
	}
	metrics.RecordEvent(ctx, eventName, kvPairs)
}
