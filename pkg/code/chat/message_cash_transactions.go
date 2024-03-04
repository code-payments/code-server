package chat

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/chat"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

// Refactored to streamline error handling and make the flow more concise.
func SendCashTransactionsExchangeMessage(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) error {
	messageId, verbByMessageReceiver, err := determineVerbsAndMessageID(ctx, data, intentRecord)
	if err != nil {
		return errors.Wrap(err, "determining verbs and message ID failed")
	}

	for account, verb := range verbByMessageReceiver {
		if err := sendExchangeMessage(ctx, data, intentRecord, account, verb, messageId); err != nil {
			return errors.Wrapf(err, "sending exchange message for account %s failed", account)
		}
	}

	return nil
}

// Streamlines the verb and message ID determination process.
func determineVerbsAndMessageID(ctx context.Context, data code_data.Provider, intentRecord *intent.Record) (string, map[string]chatpb.ExchangeDataContent_Verb, error) {
	verbByMessageReceiver := make(map[string]chatpb.ExchangeDataContent_Verb)
	messageId := getMessageID(intentRecord)

	// Simplified switch case by removing redundant context and data params
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		handleSendPrivatePayment(intentRecord, verbByMessageReceiver)
	case intent.SendPublicPayment:
		handleSendPublicPayment(intentRecord, verbByMessageReceiver)
	case intent.ReceivePaymentsPublicly:
		handleReceivePaymentsPublicly(intentRecord, verbByMessageReceiver)
	case intent.MigrateToPrivacy2022, intent.ExternalDeposit:
		handleGenericIntentTypes(intentRecord, verbByMessageReceiver)
	}

	return messageId, verbByMessageReceiver, nil
}

// Helper function to extract message ID with a focus on ExternalDeposit case.
func getMessageID(intentRecord *intent.Record) string {
	if intentRecord.IntentType == intent.ExternalDeposit {
		return strings.Split(intentRecord.IntentId, "-")[0]
	}
	return intentRecord.IntentId
}
// handleSendPrivatePayment processes private payment intents, adjusting verbs based on the payment's characteristics.
func handleSendPrivatePayment(intentRecord *intent.Record, verbByMessageReceiver map[string]chatpb.ExchangeDataContent_Verb) {
    if intentRecord.SendPrivatePaymentMetadata.IsMicroPayment {
        // Logic for micro payments; typically, these might be handled differently, perhaps not requiring a chat message.
        // Placeholder for micro payment handling; specifics depend on business logic.
    } else if intentRecord.SendPrivatePaymentMetadata.IsWithdrawal {
        verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_WITHDREW
        if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
            verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
        }
    } else if intentRecord.SendPrivatePaymentMetadata.IsRemoteSend {
        verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_SENT
    } else {
        // Default case for private payments that don't fit the above categories.
        verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_GAVE
        if len(intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount) > 0 {
            verbByMessageReceiver[intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_RECEIVED
        }
    }
}

// handleSendPublicPayment processes public payment intents, determining the appropriate verbs for the action.
func handleSendPublicPayment(intentRecord *intent.Record, verbByMessageReceiver map[string]chatpb.ExchangeDataContent_Verb) {
    if intentRecord.SendPublicPaymentMetadata.IsWithdrawal {
        verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_WITHDREW
        if len(intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount) > 0 {
            verbByMessageReceiver[intentRecord.SendPublicPaymentMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
        }
    }
    // Additional cases for public payments can be added here as needed.
}

// handleReceivePaymentsPublicly processes intents related to publicly receiving payments, assigning verbs accordingly.
func handleReceivePaymentsPublicly(intentRecord *intent.Record, verbByMessageReceiver map[string]chatpb.ExchangeDataContent_Verb) {
    if intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
        if intentRecord.ReceivePaymentsPubliclyMetadata.IsReturned {
            verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_RETURNED
        } else {
            verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_RECEIVED
        }
    }
    // Additional logic for handling other scenarios of publicly receiving payments can be included here.
}

// handleGenericIntentTypes handles intents that follow a generic processing pattern, such as MigrateToPrivacy2022 and ExternalDeposit.
func handleGenericIntentTypes(intentRecord *intent.Record, verbByMessageReceiver map[string]chatpb.ExchangeDataContent_Verb) {
    if intentRecord.IntentType == intent.MigrateToPrivacy2022 && intentRecord.MigrateToPrivacy2022Metadata.Quantity > 0 {
        verbByMessageReceiver[intentRecord.InitiatorOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
    } else if intentRecord.IntentType == intent.ExternalDeposit {
        verbByMessageReceiver[intentRecord.ExternalDepositMetadata.DestinationOwnerAccount] = chatpb.ExchangeDataContent_DEPOSITED
    }
    // This function can be extended to handle additional generic intent types as they are introduced.
}


// Note: The `getExchangeDataFromIntent` and `newProtoChatMessage` functions are found in /util.go
