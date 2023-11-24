package push

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

type dataPushType string

const (
	dataPushTypeKey = "code_notification_type"

	chatMessageDataPush dataPushType = "ChatMessage"
)

// sendDataPushNotificationToOwner is a generic utility for sending data push
// notification to the devices linked to an owner account.
//
// todo: Duplicated code with other send push utitilies
func sendDataPushNotificationToOwner(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
	notificationType dataPushType,
	kvs map[string]string,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "sendDataPushNotificationToOwner",
		"owner":  owner.PublicKey().ToBase58(),
	})

	kvs[dataPushTypeKey] = string(notificationType)

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
		err := pusher.SendDataPush(
			ctx,
			pushTokenRecord.PushToken,
			kvs,
		)

		if err != nil {
			log.WithError(err).Warn("failure sending push notification")
			onPushError(ctx, data, pusher, pushTokenRecord)
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}
	return nil
}
