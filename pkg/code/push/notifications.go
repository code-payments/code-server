package push

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/localization"
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
	chatRecord, err := data.GetChatById(ctx, chat.GetChatId(chat_util.CashTransactionsName, owner.PublicKey().ToBase58(), true))
	switch err {
	case nil:
		if chatRecord.IsMuted {
			return nil
		}
	case chat.ErrChatNotFound:
	default:
		log.WithError(err).Warn("failure getting chat record")
		return errors.Wrap(err, "error getting chat record")
	}

	titleKey := localization.PushTitleDepositReceived
	bodyKey := localization.PushSubtitleDepositReceived
	kinAmountArg := fmt.Sprintf("%d", kin.FromQuarks(quarks))
	return sendLocalizedPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		titleKey,
		bodyKey,
		kinAmountArg,
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
	chatRecord, err := data.GetChatById(ctx, chat.GetChatId(chat_util.CashTransactionsName, owner.PublicKey().ToBase58(), true))
	switch err {
	case nil:
		if chatRecord.IsMuted {
			return nil
		}
	case chat.ErrChatNotFound:
	default:
		log.WithError(err).Warn("failure getting chat record")
		return errors.Wrap(err, "error getting chat record")
	}

	titleKey := localization.PushTitleKinReturned
	bodyKey := localization.PushSubtitleKinReturned
	amountArg := getAmountArg(
		originalGiftCardIssuedIntent.SendPrivatePaymentMetadata.NativeAmount,
		originalGiftCardIssuedIntent.SendPrivatePaymentMetadata.ExchangeCurrency,
	)
	return sendLocalizedPushNotificationToOwner(
		ctx,
		data,
		pusher,
		owner,
		titleKey,
		bodyKey,
		amountArg,
	)
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

	chatProperties, ok := chat_util.InternalChatProperties[chatTitle]
	if ok {
		chatTitle = chatProperties.TitleLocalizationKey
	}

	// Best-effort try to update the badge count before pushing message content
	//
	// Note: Only chat messages generate badge counts
	err := UpdateBadgeCount(ctx, data, pusher, owner)
	if err != nil {
		log.WithError(err).Warn("failure updating badge count on device")
	}

	var anyErrorPushingContent bool
	for _, content := range chatMessage.Content {
		marshalledContent, err := proto.Marshal(content)
		if err != nil {
			log.WithError(err).Warn("failure marshalling chat content")
			return err
		}

		kvs := map[string]string{
			"chat_title":      chatTitle,
			"message_content": base64.StdEncoding.EncodeToString(marshalledContent),
		}

		err = sendDataPushNotificationToOwner(
			ctx,
			data,
			pusher,
			owner,
			chatMessageDataPush,
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
