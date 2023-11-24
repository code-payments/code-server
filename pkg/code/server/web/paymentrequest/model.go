package paymentrequest

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kikcode"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/solana"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/limit"
)

type trustedPaymentRequest struct {
	currency     currency_lib.Code
	nativeAmount float64
	destination  *common.Account

	kikCodePayload       *kikcode.Payload
	privateRendezvousKey *common.Account
}

func newTrustedPaymentRequest(
	currency currency_lib.Code,
	nativeAmount float64,
	destination *common.Account,
	idempotencyKey kikcode.IdempotencyKey,
) (*trustedPaymentRequest, error) {
	kikCodePayload, err := kikcode.NewPayloadFromFiatAmount(kikcode.PaymentRequest, currency, nativeAmount, idempotencyKey)
	if err != nil {
		return nil, err
	}

	privateRendezvousKey, err := common.NewAccountFromPrivateKeyBytes(kikCodePayload.ToRendezvousKey())
	if err != nil {
		return nil, err
	}

	return &trustedPaymentRequest{
		currency:     currency,
		nativeAmount: nativeAmount,
		destination:  destination,

		kikCodePayload:       kikCodePayload,
		privateRendezvousKey: privateRendezvousKey,
	}, nil
}

func newTrustedPaymentRequestFromHttpContext(r *http.Request) (*trustedPaymentRequest, error) {
	destinationQueryParam := r.URL.Query()["destination"]
	currencyQueryParam := r.URL.Query()["currency"]
	amountQueryParam := r.URL.Query()["amount"]
	idempotencyKeyQueryParam := r.URL.Query()["idempotency"]

	if len(destinationQueryParam) < 1 {
		return nil, errors.New("destination query parameter missing")
	}

	if len(currencyQueryParam) < 1 {
		return nil, errors.New("currency query parameter missing")
	}

	if len(amountQueryParam) < 1 {
		return nil, errors.New("amount query parameter missing")
	}

	destination, err := common.NewAccountFromPublicKeyString(destinationQueryParam[0])
	if err != nil {
		return nil, errors.New("destination is not a public key")
	}

	currency := currency_lib.Code(strings.ToLower(currencyQueryParam[0]))
	limits, ok := limit.MicroPaymentLimits[currency]
	if !ok {
		return nil, errors.Errorf("%s currency is not currently supported", currency)
	}

	amount, err := strconv.ParseFloat(amountQueryParam[0], 64)
	if err != nil {
		return nil, errors.New("amount is not a number")
	} else if amount > limits.Max {
		return nil, errors.Errorf("%s currency has a maximum amount of %.2f", currency, limits.Max)
	} else if amount < limits.Min {
		return nil, errors.Errorf("%s currency has a minimum amount of %.2f", currency, limits.Min)
	}

	idempotencyKey := kikcode.GenerateRandomIdempotencyKey()
	if len(idempotencyKeyQueryParam) > 0 {
		optionalIdempotencyKey, err := base64.RawURLEncoding.DecodeString(idempotencyKeyQueryParam[0])
		if err != nil {
			return nil, errors.New("idempotency key is not valid base64")
		}
		if len(optionalIdempotencyKey) != len(idempotencyKey) {
			return nil, errors.Errorf("idempotency key must be %d bytes long", len(idempotencyKey))
		}
		copy(idempotencyKey[:], optionalIdempotencyKey)
	}

	return newTrustedPaymentRequest(
		currency,
		amount,
		destination,
		idempotencyKey,
	)
}

func (r *trustedPaymentRequest) ToKikCodePayload() *kikcode.Payload {
	return r.kikCodePayload
}

func (r *trustedPaymentRequest) ToProtoMessage() *messagingpb.Message {
	return getRequestToReceiveBillMessage(
		r.currency,
		r.nativeAmount,
		r.destination,
	)
}

func (r *trustedPaymentRequest) GetIdempotencyKey() kikcode.IdempotencyKey {
	return r.kikCodePayload.GetIdempotencyKey()
}

func (r *trustedPaymentRequest) GetPrivateRendezvousKey() *common.Account {
	return r.privateRendezvousKey
}

type trustlessPaymentRequest struct {
	currency     currency_lib.Code
	nativeAmount float64
	destination  *common.Account

	publicRendezvousKey *common.Account
	clientSignature     solana.Signature // For a messagingpb.RequestToReceiveBill

	webhookUrl *string
}

func newTrustlessPaymentRequest(
	currency currency_lib.Code,
	nativeAmount float64,
	destination *common.Account,

	publicRendezvousKey *common.Account,
	clientSignature solana.Signature,

	webhookUrl *string,
) (*trustlessPaymentRequest, error) {
	return &trustlessPaymentRequest{
		currency:     currency,
		nativeAmount: nativeAmount,
		destination:  destination,

		publicRendezvousKey: publicRendezvousKey,
		clientSignature:     clientSignature,

		webhookUrl: webhookUrl,
	}, nil
}

func newTrustlessPaymentRequestFromHttpContext(r *http.Request) (*trustlessPaymentRequest, error) {
	httpRequestBody := struct {
		Intent    string  `json:"intent"`
		Message   string  `json:"message"`
		Signature string  `json:"signature"`
		Webhook   *string `json:"webhook"`
	}{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &httpRequestBody)
	if err != nil {
		return nil, err
	}

	rendezvousKey, err := common.NewAccountFromPublicKeyString(httpRequestBody.Intent)
	if err != nil {
		return nil, errors.New("intent is not a public key")
	}

	var messageProto messagingpb.RequestToReceiveBill
	messageBytes, err := base64.RawURLEncoding.DecodeString(httpRequestBody.Message)
	if err != nil {
		return nil, errors.New("message not valid base64")
	}
	err = proto.Unmarshal(messageBytes, &messageProto)
	if err != nil {
		return nil, errors.New("message bytes is not a RequestToReceiveBill")
	} else if err := messageProto.Validate(); err != nil {
		return nil, errors.Wrap(err, "message failed proto validation")
	}

	var signature solana.Signature
	decodedSignature, err := base58.Decode(httpRequestBody.Signature)
	if err != nil || len(decodedSignature) != len(signature) {
		return nil, errors.New("signature is invalid")
	}
	copy(signature[:], decodedSignature)

	destination, err := common.NewAccountFromProto(messageProto.RequestorAccount)
	if err != nil {
		return nil, errors.New("destination is not a public key")
	}

	var currency currency_lib.Code
	var amount float64
	switch typed := messageProto.ExchangeData.(type) {
	case *messagingpb.RequestToReceiveBill_Exact:
		currency = currency_lib.Code(strings.ToLower(typed.Exact.Currency))
		amount = float64(kin.FromQuarks(typed.Exact.Quarks)) // Because of minimum bucket sizes

		if currency != currency_lib.KIN {
			return nil, errors.New("exact exchange data is reserved for kin only")
		}

		if typed.Exact.ExchangeRate != 1.0 {
			return nil, errors.New("kin exchange rate must be 1.0")
		} else if kin.ToQuarks(uint64(amount)) != typed.Exact.Quarks {
			return nil, errors.New("kin amount cannot be fractional")
		} else if amount != typed.Exact.NativeAmount {
			return nil, errors.New("kin amount doesn't match quarks")
		}
	case *messagingpb.RequestToReceiveBill_Partial:
		currency = currency_lib.Code(strings.ToLower(typed.Partial.Currency))
		amount = typed.Partial.NativeAmount

		if currency == currency_lib.KIN {
			return nil, errors.New("partial exchange data is reserved for fiat currencies")
		}
	default:
		return nil, errors.New("exchange data not provided")
	}

	limits, ok := limit.MicroPaymentLimits[currency]
	if !ok {
		return nil, errors.Errorf("%s currency is not currently supported", currency)
	} else if amount > limits.Max {
		return nil, errors.Errorf("%s currency has a maximum amount of %.2f", currency, limits.Max)
	} else if amount < limits.Min {
		return nil, errors.Errorf("%s currency has a minimum amount of %.2f", currency, limits.Min)
	}

	if httpRequestBody.Webhook != nil {
		err = netutil.ValidateHttpUrl(*httpRequestBody.Webhook, true, false)
		if err != nil {
			return nil, err
		}
	}

	return newTrustlessPaymentRequest(
		currency,
		amount,
		destination,

		rendezvousKey,
		signature,

		httpRequestBody.Webhook,
	)
}

func (r *trustlessPaymentRequest) GetPublicRendezvousKey() *common.Account {
	return r.publicRendezvousKey
}

func (r *trustlessPaymentRequest) GetClientSignature() solana.Signature {
	return r.clientSignature
}

func (r *trustlessPaymentRequest) ToProtoMessage() *messagingpb.Message {
	return getRequestToReceiveBillMessage(
		r.currency,
		r.nativeAmount,
		r.destination,
	)
}

func getRequestToReceiveBillMessage(
	currency currency_lib.Code,
	nativeAmount float64,
	destination *common.Account,
) *messagingpb.Message {
	if currency == currency_lib.KIN {
		quarks := kin.ToQuarks(uint64(nativeAmount))
		if int(100.0*nativeAmount)%100.0 != 0 {
			quarks += kin.ToQuarks(1)
		}

		return &messagingpb.Message{
			Kind: &messagingpb.Message_RequestToReceiveBill{
				RequestToReceiveBill: &messagingpb.RequestToReceiveBill{
					RequestorAccount: destination.ToProto(),
					ExchangeData: &messagingpb.RequestToReceiveBill_Exact{
						Exact: &transactionpb.ExchangeData{
							Currency:     string(currency),
							ExchangeRate: 1.0,
							NativeAmount: nativeAmount,
							Quarks:       quarks,
						},
					},
				},
			},
		}
	}

	return &messagingpb.Message{
		Kind: &messagingpb.Message_RequestToReceiveBill{
			RequestToReceiveBill: &messagingpb.RequestToReceiveBill{
				RequestorAccount: destination.ToProto(),
				ExchangeData: &messagingpb.RequestToReceiveBill_Partial{
					Partial: &transactionpb.ExchangeDataWithoutRate{
						Currency:     string(currency),
						NativeAmount: nativeAmount,
					},
				},
			},
		},
	}
}
