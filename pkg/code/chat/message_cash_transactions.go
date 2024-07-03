package chat

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// SendCashTransactionsExchangeMessage sends a message to the Cash Transactions
// chat with exchange data content related to the submitted intent. Intents that
// don't belong in the Cash Transactions chat will be ignored.
//
// Note: Tests covered in SubmitIntent history tests
func SendCashTransactionsExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) error {
	messageId := intentRecord.IntentId

	exchangeData, ok := getExchangeDataFromIntent(intentRecord)
	if !ok {
		return nil
	}

	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
			// Micro payment messages exist in merchant domain-specific chats
			return nil
		} else if intentRecord.SendPrivatePaymentMetadata.IsTip {
			// Tip messages exist in a tip-specific chat
			return nil
		} else if intentRecord.SendPrivatePaymentMetadata.IsWithdrawal {
			if intentRecord.InitiatorOwnerAccount == intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount {
				// This is a top up for a public withdawal
				return nil
			}

			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_WITHDREW
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return err
				} else if destinationAccountInfoRecord.AccountType != commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts payments will show up in the verified
					// merchant chat
					verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
				}
			}
		} else if intentRecord.SendPrivatePaymentMetadata.IsRemoteSend {
			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_SENT
		} else {
			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_GAVE
			if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
				verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_RECEIVED
			}
		}

	case intent.SendPublicPayment:
		if intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
			if intentRecord.InitiatorOwnerAccount == intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount {
				// This is an internal movement of funds across the same Code user's public accounts
				return nil
			}

			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_WITHDREW
			if len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) > 0 {
				destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount)
				if err != nil {
					return err
				} else if destinationAccountInfoRecord.AccountType != commonpb.AccountType_RELATIONSHIP {
					// Relationship accounts payments will show up in the verified
					// merchant chat
					verbByMessageReceiver[intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
				}
			}
		}

	case intent.ReceivePaymentsPublicly:
		if intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
			if intentRecord.ReceivePaymentsPubliclyMetadata.IsReturned {
				verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_RETURNED
			} else if intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard {
				giftCardIssuedIntentRecord, err := data.GetOriginalGiftCardIssuedIntent(ctx, intentRecord.ReceivePaymentsPubliclyMetadata.Source)
				if err != nil {
					return errors.Wrap(err, "error getting original gift card issued intent")
				}

				chatId := chat_v1.GetChatId(CashTransactionsName, giftCardIssuedIntentRecord.InitiatorOwnerAccount, true)

				err = data.DeleteChatMessageV1(ctx, chatId, giftCardIssuedIntentRecord.IntentId)
				if err != nil {
					return errors.Wrap(err, "error deleting chat message")
				}
				return nil
			} else {
				verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_RECEIVED
			}
		}

	case intent.MigrateToPrivacy2022:
		if intentRecord.MigrateToPrivacy2022Metadata.Quantity > 0 {
			verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
		}

	case intent.ExternalDeposit:
		messageId = strings.Split(messageId, "-")[0]
		destinationAccountInfoRecord, err := data.GetAccountInfoByTokenAddress(ctx, intentRecord.ExternalDepositMetadata.DestinationTokenAccount)
		if err != nil {
			return err
		} else if destinationAccountInfoRecord.AccountType != commonpb.AccountType_RELATIONSHIP {
			// Relationship accounts payments will show up in the verified
			// merchant chat
			verbByMessageReceiver[intentRecord.ExternalDepositMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
		}

	default:
		return nil
	}

	for account, verb := range verbByMessageReceiver {
		receiver, err := common.NewAccountFromPublicKeyString(account)
		if err != nil {
			return err
		}

		content := []*chatpb.Content{
			{
				Type: &chatpb.Content_ExchangeData{
					ExchangeData: &chatpb.ExchangeDataContent{
						Verb: verb,
						ExchangeData: &chatpb.ExchangeDataContent_Exact{
							Exact: exchangeData,
						},
					},
				},
			},
		}
		protoMessage, err := newProtoChatMessage(messageId, content, intentRecord.CreatedAt)
		if err != nil {
			return errors.Wrap(err, "error creating proto chat message")
		}

		_, err = SendChatMessage(
			ctx,
			data,
			CashTransactionsName,
			chat_v1.ChatTypeInternal,
			true,
			receiver,
			protoMessage,
			true,
		)
		if err != nil && err != chat_v1.ErrMessageAlreadyExists {
			return errors.Wrap(err, "error persisting chat message")
		}
	}

	return nil
}
