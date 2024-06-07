package push

import (
	"context"
	"encoding/base64"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/localization"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

// SendDepositPushNotification sends a push notification for received deposits
func SendDepositPushNotification(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	vault *common.Account,
	quarks uint64,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "SendDepositPushNotification",
		"vault":  vault.PublicKey().ToBase58(),
		"quarks": quarks,
	})

	if quarks < kin.ToQuarks(1) {
		return nil
	}

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, vault.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting account info record")
		return errors.Wrap(err, "error getting account info record")
	}

	if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
		return nil
	}

	owner, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return errors.Wrap(err, "invalid owner account")
	}

	// Legacy push notification still considers chat mute state
	//
	// todo: Proper migration to chat system
	chatRecord, err := data.GetChatByIdV1(ctx, chat_v1.GetChatId(chat_util.CashTransactionsName, owner.PublicKey().ToBase58(), true))
	switch err {
	case nil:
		if chatRecord.IsMuted {
			return nil
		}
	case chat_v1.ErrChatNotFound:
	default:
		log.WithError(err).Warn("failure getting chat record")
		return errors.Wrap(err, "error getting chat record")
	}

	locale, err := data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
		return err
	}

	localizedAmount, err := localization.FormatFiat(locale, currency_lib.KIN, float64(kin.FromQuarks(quarks)), false)
	if err != nil {
		return nil
	}

	localizedPushTitle, err := localization.Localize(locale, localization.PushTitleDepositReceived)
	if err != nil {
		return nil
	}

	localizedPushBody, err := localization.Localize(locale, localization.PushSubtitleDepositReceived, localizedAmount)
	if err != nil {
		return nil
	}

	return sendBasicPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		localizedPushTitle,
		localizedPushBody,
	)
}

// SendGiftCardReturnedPushNotification sends a push notification when a gift card
// has expired and the balance has returned to the issuing user
func SendGiftCardReturnedPushNotification(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	giftCardVault *common.Account,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "SendGiftCardReturnedPushNotification",
		"vault":  giftCardVault.PublicKey().ToBase58(),
	})

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, giftCardVault.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting account info record")
		return errors.Wrap(err, "error getting account info record")
	}

	if accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		return nil
	}

	originalGiftCardIssuedIntent, err := data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVault.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting original gift card issued intent")
		return errors.Wrap(err, "error getting original gift card issued intent")
	}

	owner, err := common.NewAccountFromPublicKeyString(originalGiftCardIssuedIntent.InitiatorOwnerAccount)
	if err != nil {
		return errors.Wrap(err, "invalid owner")
	}

	// Legacy push notification still considers chat mute state
	//
	// todo: Proper migration to chat system
	chatRecord, err := data.GetChatByIdV1(ctx, chat_v1.GetChatId(chat_util.CashTransactionsName, owner.PublicKey().ToBase58(), true))
	switch err {
	case nil:
		if chatRecord.IsMuted {
			return nil
		}
	case chat_v1.ErrChatNotFound:
	default:
		log.WithError(err).Warn("failure getting chat record")
		return errors.Wrap(err, "error getting chat record")
	}

	locale, err := data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
		return err
	}

	localizedAmount, err := localization.FormatFiat(
		locale,
		originalGiftCardIssuedIntent.SendPrivatePaymentMetadata.ExchangeCurrency,
		originalGiftCardIssuedIntent.SendPrivatePaymentMetadata.NativeAmount,
		true,
	)
	if err != nil {
		return nil
	}

	localizedPushTitle, err := localization.Localize(locale, localization.PushTitleKinReturned)
	if err != nil {
		return nil
	}

	localizedPushBody, err := localization.Localize(locale, localization.PushSubtitleKinReturned, localizedAmount)
	if err != nil {
		return nil
	}

	return sendBasicPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		localizedPushTitle,
		localizedPushBody,
	)
}

// SendTwitterAccountConnectedPushNotification sends a push notification for newly
// connected Twitter accounts
func SendTwitterAccountConnectedPushNotification(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	tipAccount *common.Account,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method":      "SendTwitterAccountConnectedPushNotification",
		"tip_account": tipAccount.PublicKey().ToBase58(),
	})

	accountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, tipAccount.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting account info record")
		return errors.Wrap(err, "error getting account info record")
	}
	if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
		return nil
	}

	owner, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return errors.Wrap(err, "invalid owner account")
	}

	locale, err := data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
		return err
	}

	localizedPushTitle, err := localization.Localize(locale, localization.PushTitleTwitterAccountConnected)
	if err != nil {
		return nil
	}

	localizedPushBody, err := localization.Localize(locale, localization.PushSubtitleTwitterAccountConnected)
	if err != nil {
		return nil
	}

	return sendBasicPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		localizedPushTitle,
		localizedPushBody,
	)
}

// SendTriggerSwapRpcPushNotification sends a data push to trigger the Swap RPC
func SendTriggerSwapRpcPushNotification(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "SendTriggerSwapRpcPushNotification",
		"owner":  owner.PublicKey().ToBase58(),
	})

	err := sendRawDataPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		executeSwapDataPush,
		make(map[string]string),
	)
	if err != nil {
		log.WithError(err).Warn("failure sending data push notification")
		return err
	}

	return nil
}

// SendChatMessagePushNotification sends a push notification for chat messages
func SendChatMessagePushNotification(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	chatTitle string,
	owner *common.Account,
	chatMessage *chatpb.ChatMessage,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "SendChatMessagePushNotification",
		"owner":  owner.PublicKey().ToBase58(),
		"chat":   chatTitle,
	})

	// Best-effort try to update the badge count before pushing message content
	//
	// Note: Only chat messages generate badge counts
	err := UpdateBadgeCount(ctx, data, pusher, owner)
	if err != nil {
		log.WithError(err).Warn("failure updating badge count on device")
	}

	locale, err := data.GetUserLocale(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting user locale")
		return err
	}

	var localizedPushTitle string

	chatProperties, ok := chat_util.InternalChatProperties[chatTitle]
	if ok {
		localized, err := localization.Localize(locale, chatProperties.TitleLocalizationKey)
		if err != nil {
			return nil
		}
		localizedPushTitle = localized
	} else {
		domainDisplayName, err := thirdparty.GetDomainDisplayName(chatTitle)
		if err == nil {
			localizedPushTitle = domainDisplayName
		} else {
			return nil
		}
	}

	var anyErrorPushingContent bool
	for _, content := range chatMessage.Content {
		var contentToPush *chatpb.Content
		switch typedContent := content.Type.(type) {
		case *chatpb.Content_ServerLocalized:
			localizedPushBody, err := localization.Localize(locale, typedContent.ServerLocalized.KeyOrText)
			if err != nil {
				continue
			}

			contentToPush = &chatpb.Content{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						KeyOrText: localizedPushBody,
					},
				},
			}
		case *chatpb.Content_ExchangeData:
			var currencyCode currency_lib.Code
			var nativeAmount float64
			if typedContent.ExchangeData.GetExact() != nil {
				exchangeData := typedContent.ExchangeData.GetExact()
				currencyCode = currency_lib.Code(exchangeData.Currency)
				nativeAmount = exchangeData.NativeAmount
			} else {
				exchangeData := typedContent.ExchangeData.GetPartial()
				currencyCode = currency_lib.Code(exchangeData.Currency)
				nativeAmount = exchangeData.NativeAmount
			}

			localizedPushBody, err := localization.LocalizeFiatWithVerb(
				locale,
				typedContent.ExchangeData.Verb,
				currencyCode,
				nativeAmount,
				true,
			)
			if err != nil {
				continue
			}

			contentToPush = &chatpb.Content{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						KeyOrText: localizedPushBody,
					},
				},
			}
		case *chatpb.Content_NaclBox, *chatpb.Content_Text:
			contentToPush = content
		case *chatpb.Content_ThankYou:
			contentToPush = &chatpb.Content{
				Type: &chatpb.Content_ServerLocalized{
					ServerLocalized: &chatpb.ServerLocalizedContent{
						// todo: localize this
						KeyOrText: "🙏 They thanked you for their tip",
					},
				},
			}
		}

		if contentToPush == nil {
			continue
		}

		marshalledContent, err := proto.Marshal(contentToPush)
		if err != nil {
			log.WithError(err).Warn("failure marshalling chat content")
			return err
		}

		kvs := map[string]string{
			"chat_title":      localizedPushTitle,
			"message_content": base64.StdEncoding.EncodeToString(marshalledContent),
		}

		err = sendMutableNotificationToOwner(
			ctx,
			data,
			pusher,
			owner,
			chatMessageDataPush,
			chatTitle,
			kvs,
		)
		if err != nil {
			anyErrorPushingContent = true
			log.WithError(err).Warn("failure sending data push notification")
		}
	}

	if anyErrorPushingContent {
		return errors.New("at least one piece of content failed to push")
	}
	return nil
}
