package webhook

import (
	"context"
	"strings"

	"github.com/pkg/errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
)

type jsonPayloadProvider func(ctx context.Context, data code_data.Provider, record *webhook.Record) (map[string]interface{}, error)

var jsonPayloadProviders = map[webhook.Type]jsonPayloadProvider{
	webhook.TypeIntentSubmitted: intentSubmittedJsonPayloadProvider,
	webhook.TypeTest:            testJsonPayloadProvider,
}

func intentSubmittedJsonPayloadProvider(ctx context.Context, data code_data.Provider, webhookRecord *webhook.Record) (map[string]interface{}, error) {
	if webhookRecord.Type != webhook.TypeIntentSubmitted {
		return nil, errors.New("invalid webhook type")
	}

	kvs := make(map[string]interface{})

	paymentRequestRecord, err := data.GetPaymentRequest(ctx, webhookRecord.WebhookId)
	if err != nil {
		return nil, errors.Wrap(err, "error getting payment request record")
	}

	intentRecord, err := data.GetIntent(ctx, webhookRecord.WebhookId)
	if err != nil {
		return nil, errors.Wrap(err, "error getting intent record")
	} else if intentRecord.State == intent.StateRevoked {
		return nil, errors.New("intent is revoked")
	}

	if paymentRequestRecord.RequiresPayment() {
		var currency currency_lib.Code
		var amount float64
		var exchangeRate float64
		var quarks uint64
		var destination string
		var isMicroPayment bool
		switch intentRecord.IntentType {
		case intent.SendPrivatePayment:
			currency = intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency
			amount = intentRecord.SendPrivatePaymentMetadata.NativeAmount
			exchangeRate = intentRecord.SendPrivatePaymentMetadata.ExchangeRate
			quarks = intentRecord.SendPrivatePaymentMetadata.Quantity
			destination = intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount
			isMicroPayment = intentRecord.SendPrivatePaymentMetadata.IsMicroPayment
		default:
			return nil, errors.Errorf("%d intent type is not supported", intentRecord.IntentType)
		}
		if !isMicroPayment {
			return nil, errors.New("intent is not a micro payment")
		}

		// todo: Use a more efficient DB query or stuff fee metadata directly in the intent record
		actionRecords, err := data.GetAllActionsByIntent(ctx, intentRecord.IntentId)
		if err != nil {
			return nil, errors.Wrap(err, "error getting action records")
		}
		var thirdPartyPaymentAction *action.Record
		for _, actionRecord := range actionRecords {
			if actionRecord.ActionType != action.NoPrivacyWithdraw {
				continue
			}

			if *actionRecord.Destination == destination {
				thirdPartyPaymentAction = actionRecord
				break
			}
		}
		if thirdPartyPaymentAction == nil {
			return nil, errors.New("third party payment action not found")
		}

		kvs = map[string]interface{}{
			"intent":       intentRecord.IntentId,
			"currency":     strings.ToUpper(string(currency)),
			"amount":       amount,
			"exchangeRate": exchangeRate,
			"quarks":       quarks,
			"fees":         quarks - *thirdPartyPaymentAction.Quantity,
			"destination":  destination,
			"state":        "SUBMITTED",
		}
	}

	if paymentRequestRecord.HasLogin() {
		relationshipAccountInfoRecord, err := data.GetRelationshipAccountInfoByOwnerAddress(ctx, intentRecord.InitiatorOwnerAccount, *paymentRequestRecord.Domain)
		if err == nil {
			kvs["user"] = relationshipAccountInfoRecord.AuthorityAccount
		} else if err != account.ErrAccountInfoNotFound {
			return nil, errors.Wrap(err, "error querying for relationship account")
		}
	}

	return kvs, nil
}

func testJsonPayloadProvider(ctx context.Context, data code_data.Provider, webhookRecord *webhook.Record) (map[string]interface{}, error) {
	if webhookRecord.Type != webhook.TypeTest {
		return nil, errors.New("invalid webhook type")
	}

	return map[string]interface{}{
		"string": "string_value",
		"int":    42,
		"float":  123.45,
		"bool":   true,
	}, nil
}
