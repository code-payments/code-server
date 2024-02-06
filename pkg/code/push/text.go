package push

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_data "github.com/code-payments/code-server/pkg/code/data/push"
	"github.com/code-payments/code-server/pkg/code/localization"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

// sendLocalizedPushNotificationToOwner is a generic utility for sending a localized
// push notification to the devices linked to an owner account. Keys can be provided
// in either the iOS or Android format, and will be translated accordingly.
//
// todo: Duplicated code with other send push utitilies
func sendLocalizedPushNotificationToOwner(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
	titleKey, bodyKey string,
	bodyArgs ...string,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "sendLocalizedPushNotificationToOwner",
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
		var err error
		switch pushTokenRecord.TokenType {
		case push_data.TokenTypeFcmApns:
			err = pusher.SendLocalizedAPNSPush(
				ctx,
				pushTokenRecord.PushToken,
				localization.GetIosLocalizationKey(titleKey),
				localization.GetIosLocalizationKey(bodyKey),
				bodyArgs...,
			)
		case push_data.TokenTypeFcmAndroid:
			err = pusher.SendLocalizedAndroidPush(
				ctx,
				pushTokenRecord.PushToken,
				localization.GetAndroidLocalizationKey(titleKey),
				localization.GetAndroidLocalizationKey(bodyKey),
				bodyArgs...,
			)
		default:
		}

		if err != nil {
			log.WithError(err).Warn("failure sending push notification")
			onPushError(ctx, data, pusher, pushTokenRecord)
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}
	return nil
}

// sendMutableNotificationToOwner is a generic utility for sending mutable
// npush otification to the devices linked to an owner account.
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

// sendBasicPushNotificationToOwner is a generic utility for sending push notification
// to the devices linked to an owner account. This should be used early in features that
// don't have localization, since titles & body are the direct English text.
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
			log.WithError(err).Warn("failure sending push notification")
			onPushError(ctx, data, pusher, pushTokenRecord)
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}
	return nil
}
