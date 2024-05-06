package push

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_data "github.com/code-payments/code-server/pkg/code/data/push"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

type dataPushType string

const (
	dataPushTypeKey = "code_notification_type"

	chatMessageDataPush dataPushType = "ChatMessage"
	executeSwapDataPush dataPushType = "ExecuteSwap"
)

// sendRawDataPushNotificationToOwner is a generic utility for sending raw data push
// notification to the devices linked to an owner account.
//
// todo: Duplicated code with other send push utitilies
func sendRawDataPushNotificationToOwner(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
	notificationType dataPushType,
	kvs map[string]string,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "sendRawDataPushNotificationToOwner",
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

// sendMutableNotificationToOwner is a generic utility for sending mutable
// push notification to the devices linked to an owner account. It's a
// special data push where the notification content is replaced by the contents
// of a kv pair payload.
//
// todo: Duplicated code with other send push utitilies
func sendMutableNotificationToOwner(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
	notificationType dataPushType,
	titleKey string,
	kvs map[string]string,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "sendMutableNotificationToOwner",
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
		var err error
		switch pushTokenRecord.TokenType {
		case push_data.TokenTypeFcmApns:
			err = pusher.SendMutableAPNSPush(
				ctx,
				pushTokenRecord.PushToken,
				titleKey,
				string(notificationType),
				titleKey, // All mutable pushes have a thread ID that's the title
				kvs,
			)
		case push_data.TokenTypeFcmAndroid:
			// todo: anything special required for Android?
			err = pusher.SendDataPush(
				ctx,
				pushTokenRecord.PushToken,
				kvs,
			)
		}

		if err != nil {
			log.WithError(err).Warn("failure sending push notification")
			onPushError(ctx, data, pusher, pushTokenRecord)
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}
	return nil
}
