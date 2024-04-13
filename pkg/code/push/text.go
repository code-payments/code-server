package push

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

// sendBasicPushNotificationToOwner is a generic utility for sending push notification
// to the devices linked to an owner account.
//
// todo: Duplicated code with other send push utitilies
func sendBasicPushNotificationToOwner(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
	title, body string,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "sendPushNotificationToOwner",
		"owner":  owner.PublicKey().ToBase58(),
	})

	pushTokenRecords, err := getPushTokensForOwner(ctx, data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting push tokens for owner")
		return err
	}

	seenPushTokens := make(map[string]struct{})
	for _, pushTokenRecord := range pushTokenRecords {
		// Dedup push tokens, since they may appear more than once per app install
		if _, ok := seenPushTokens[pushTokenRecord.PushToken]; ok {
			continue
		}

		log := log.WithField("push_token", pushTokenRecord.PushToken)

		// Try push
		err := pusher.SendPush(
			ctx,
			pushTokenRecord.PushToken,
			title,
			body,
		)

		if err != nil {
			isValid, onPushErr := onPushError(ctx, data, pusher, pushTokenRecord)
			log.WithError(err).
				WithFields(logrus.Fields{
					"cleanup_error": onPushErr,
					"is_valid":      isValid,
				}).Warn("failed to send push notification (best effort)")
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}
	return nil
}
