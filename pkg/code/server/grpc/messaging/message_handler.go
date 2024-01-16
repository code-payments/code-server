package messaging

import (
	"bytes"
	"context"
	"math"
	"time"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/code/auth"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	exchange_rate_util "github.com/code-payments/code-server/pkg/code/exchangerate"
	"github.com/code-payments/code-server/pkg/code/limit"
	"github.com/code-payments/code-server/pkg/code/thirdparty"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/pointer"
)

// MessageHandler provides message-specific in addition to the generic message
// handling flows. Implementations are responsible for determining whether a
// message can be allowed to be retried.
type MessageHandler interface {
	// Validate validates a message, which determines whether it should be
	// allowed to be sent and persisted
	Validate(ctx context.Context, rendezvous *common.Account, message *messagingpb.Message) error

	// RequiresActiveStream determines whether a message can only be sent if an
	// active stream is available. If true, then the message must also provide
	// the maximum time it expects the stream to be valid for, which is dependent
	// on the use case.
	RequiresActiveStream() (bool, time.Duration)

	// OnSuccess is called upon creating the message after validation
	OnSuccess(ctx context.Context) error
}

type RequestToGrabBillMessageHandler struct {
	data code_data.Provider
}

func NewRequestToGrabBillMessageHandler(data code_data.Provider) MessageHandler {
	return &RequestToGrabBillMessageHandler{
		data: data,
	}
}

func (h *RequestToGrabBillMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetRequestToGrabBill()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	//
	// Part 1: Requestor account must be a latest temporary incoming account
	//

	requestorAccount, err := common.NewAccountFromProto(typedMessage.RequestorAccount)
	if err != nil {
		return err
	}

	accountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, requestorAccount.PublicKey().ToBase58())
	if err == account.ErrAccountInfoNotFound || (err == nil && accountInfoRecord.AccountType != commonpb.AccountType_TEMPORARY_INCOMING) {
		return newMessageValidationError("requestor account must be a temporary incoming account")
	} else if err != nil {
		return err
	}

	latestAccountInfoRecord, err := h.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, accountInfoRecord.OwnerAccount, commonpb.AccountType_TEMPORARY_INCOMING)
	if err != nil {
		return err
	} else if accountInfoRecord.TokenAccount != latestAccountInfoRecord.TokenAccount {
		return newMessageValidationErrorf("requestor account must be latest temporary incoming account %s", accountInfoRecord.TokenAccount)
	}

	return nil
}

func (h *RequestToGrabBillMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return true, time.Minute
}

func (h *RequestToGrabBillMessageHandler) OnSuccess(ctx context.Context) error {
	return nil
}

type RequestToReceiveBillMessageHandler struct {
	conf                 *conf
	data                 code_data.Provider
	rpcSignatureVerifier *auth.RPCSignatureVerifier
	domainVerifier       thirdparty.DomainVerifier

	recordAlreadyExists bool
	recordToSave        *paymentrequest.Record
}

func NewRequestToReceiveBillMessageHandler(
	conf *conf, data code_data.Provider,
	rpcSignatureVerifier *auth.RPCSignatureVerifier,
	domainVerifier thirdparty.DomainVerifier,
) MessageHandler {
	return &RequestToReceiveBillMessageHandler{
		conf:                 conf,
		data:                 data,
		rpcSignatureVerifier: rpcSignatureVerifier,
		domainVerifier:       domainVerifier,
	}
}

func (h *RequestToReceiveBillMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetRequestToReceiveBill()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	requestorAccount, err := common.NewAccountFromProto(typedMessage.RequestorAccount)
	if err != nil {
		return err
	}

	var currency currency_lib.Code
	var nativeAmount float64
	var exchangeRate *float64
	var quarks *uint64
	switch typed := typedMessage.ExchangeData.(type) {
	case *messagingpb.RequestToReceiveBill_Exact:
		currency = currency_lib.Code(typed.Exact.Currency)
		nativeAmount = typed.Exact.NativeAmount
		exchangeRate = &typed.Exact.ExchangeRate
		quarks = &typed.Exact.Quarks

		if currency != currency_lib.KIN {
			return newMessageValidationError("exact exchange data is reserved for kin only")
		}

		if nativeAmount != math.Trunc(nativeAmount) {
			return newMessageValidationError("native amount can't include fractional kin")
		}
		if *quarks%kin.QuarksPerKin != 0 {
			return newMessageValidationError("quark amount can't include fractional kin")
		}
	case *messagingpb.RequestToReceiveBill_Partial:
		currency = currency_lib.Code(typed.Partial.Currency)
		nativeAmount = typed.Partial.NativeAmount

		if currency == currency_lib.KIN {
			return newMessageValidationError("partial exchange data is reserved for fiat currencies")
		}
	default:
		return newMessageValidationError("exchange data is nil")
	}

	//
	// Part 1: Validate the intent doesn't exist
	//

	_, err = h.data.GetIntent(ctx, rendezvous.PublicKey().ToBase58())
	if err == nil {
		return newMessageValidationError("client submitted intent")
	} else if err != intent.ErrIntentNotFound {
		return err
	}

	//
	// Part 2: Validate the payment request metadata
	//

	var asciiBaseDomain string
	if typedMessage.Domain != nil {
		asciiBaseDomain, err = thirdparty.GetAsciiBaseDomain(typedMessage.Domain.Value)
		if err != nil {
			return newMessageValidationErrorf("domain is invalid: %s", err.Error())
		}
	}

	existingRequestRecord, err := h.data.GetRequest(ctx, rendezvous.PublicKey().ToBase58())
	switch err {
	case nil:
		//
		// Part 2.1: Validate the relevant payment request details are exactly
		//           the same. This flow enables us to retry sending messages,
		//           while guaranteeing consistency without changing the intent.
		//

		if !existingRequestRecord.RequiresPayment() {
			return newMessageValidationError("original request doesn't require payment")
		}

		if *existingRequestRecord.DestinationTokenAccount != requestorAccount.PublicKey().ToBase58() {
			return newMessageValidationError("destination mismatches original request")
		}

		if *existingRequestRecord.ExchangeCurrency != string(currency) {
			return newMessageValidationError("exchange currency mismatches original request")
		}

		if *existingRequestRecord.NativeAmount != nativeAmount {
			return newMessageValidationError("native amount mismatches original request")
		}

		if exchangeRate != nil {
			if *existingRequestRecord.ExchangeRate != *exchangeRate {
				return newMessageValidationError("exchange rate mismatches original request")
			}
		}

		if quarks != nil {
			if *existingRequestRecord.Quantity != *quarks {
				return newMessageValidationError("quarks mismatches original request")
			}
		}

		if len(asciiBaseDomain) > 0 && (existingRequestRecord.Domain == nil || *existingRequestRecord.Domain != asciiBaseDomain) {
			return newMessageValidationError("domain mismatches original request")
		} else if len(asciiBaseDomain) == 0 && existingRequestRecord.Domain != nil {
			return newMessageValidationError("domain mismatches original request")
		}

		if existingRequestRecord.IsVerified && typedMessage.Verifier == nil {
			return newMessageValidationError("original request is verified")
		} else if !existingRequestRecord.IsVerified && typedMessage.Verifier != nil {
			// todo: allow an upgrade to the payment request?
			return newMessageValidationError("original request isn't verified")
		}

		h.recordAlreadyExists = true
	case paymentrequest.ErrPaymentRequestNotFound:
		//
		// Part 2.1: Requestor account must be a primary account (for trial use cases)
		//           or an external account (for real production use cases)
		//

		accountInfoRecord, err := h.data.GetAccountInfoByTokenAddress(ctx, requestorAccount.PublicKey().ToBase58())
		switch err {
		case nil:
			switch accountInfoRecord.AccountType {
			case commonpb.AccountType_PRIMARY:
			case commonpb.AccountType_RELATIONSHIP:
				if typedMessage.Verifier == nil {
					return newMessageValidationError("domain verification is required when requestor account is a relationship account")
				}

				if *accountInfoRecord.RelationshipTo != asciiBaseDomain {
					return newMessageValidationErrorf("requestor account must have a relationship with %s", asciiBaseDomain)
				}
			default:
				return newMessageValidationError("requestor account must be a code deposit account")
			}
		case account.ErrAccountInfoNotFound:
			if !h.conf.disableBlockchainChecks.Get(ctx) {
				err := validateExternalKinTokenAccountWithinMessage(ctx, h.data, requestorAccount)
				if err != nil {
					return err
				}
			}
		default:
			return err
		}

		//
		// Part 2.2: Exchange data validation
		//

		limits, ok := limit.MicroPaymentLimits[currency]
		if !ok {
			return newMessageValidationErrorf("%s currency is not currently supported", currency)
		} else if nativeAmount > limits.Max {
			return newMessageValidationErrorf("%s currency has a maximum amount of %.2f", currency, limits.Max)
		} else if nativeAmount < limits.Min {
			return newMessageValidationErrorf("%s currency has a minimum amount of %.2f", currency, limits.Min)
		}

		if typedMessage.GetExact() != nil {
			err = validateExchangeDataWithinMessage(ctx, h.data, typedMessage.GetExact())
			if err != nil {
				return err
			}
		}
	default:
		return err
	}

	//
	// Part 3: Domain validation, if provided
	//

	var isVerified bool
	if len(asciiBaseDomain) > 0 {
		if typedMessage.Verifier == nil {
			if typedMessage.Signature != nil {
				return newMessageValidationError("signature can only be set when verifying domain")
			}

			if typedMessage.RendezvousKey != nil {
				return newMessageValidationError("rendezvous key can only be set when verifying domain")
			}
		} else {
			if typedMessage.Signature == nil {
				return newMessageValidationError("signature must be set for domain verification")
			}

			if typedMessage.RendezvousKey == nil {
				return newMessageValidationError("rendezvous key must be set for domain verification")
			}

			if !bytes.Equal(rendezvous.PublicKey().ToBytes(), typedMessage.RendezvousKey.Value) {
				return newMessageValidationError("rendezvous key mismatch")
			}

			owner, err := common.NewAccountFromProto(typedMessage.Verifier)
			if err != nil {
				return err
			}

			signature := typedMessage.Signature
			typedMessage.Signature = nil
			if err := h.rpcSignatureVerifier.Authenticate(ctx, owner, typedMessage, signature); err != nil {
				return newMessageAuthenticationError("")
			}
			typedMessage.Signature = signature

			err = verifyThirdPartyDomain(ctx, h.domainVerifier, owner, typedMessage.Domain)
			if err != nil {
				return err
			}
			isVerified = true
		}
	}

	if len(asciiBaseDomain) == 0 {
		if typedMessage.Verifier != nil {
			return newMessageValidationError("verifier cannot be set when domain not provided")
		}

		if typedMessage.Signature != nil {
			return newMessageValidationError("signature cannot be set when domain not provided")
		}

		if typedMessage.RendezvousKey != nil {
			return newMessageValidationError("rendezvous key cannot be set when domain not provided")
		}
	}

	//
	// Part 4: Create the validated payment request DB record to store later,
	//         if it doesn't already exist
	//

	if !h.recordAlreadyExists {
		h.recordToSave = &paymentrequest.Record{
			Intent: rendezvous.PublicKey().ToBase58(),

			DestinationTokenAccount: pointer.String(requestorAccount.PublicKey().ToBase58()),
			ExchangeCurrency:        pointer.String(string(currency)),
			NativeAmount:            pointer.Float64(nativeAmount),
			ExchangeRate:            exchangeRate,
			Quantity:                quarks,

			CreatedAt: time.Now(),
		}

		if typedMessage.Domain != nil {
			h.recordToSave.Domain = &asciiBaseDomain
			h.recordToSave.IsVerified = isVerified
		}
	}

	return nil
}

func (h *RequestToReceiveBillMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return false, 0 * time.Minute
}

func (h *RequestToReceiveBillMessageHandler) OnSuccess(ctx context.Context) error {
	if h.recordAlreadyExists {
		return nil
	}
	return h.data.CreateRequest(ctx, h.recordToSave)
}

type ClientRejectedPaymentMessageHandler struct {
}

func NewClientRejectedPaymentMessageHandler() MessageHandler {
	return &ClientRejectedPaymentMessageHandler{}
}

func (h *ClientRejectedPaymentMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetClientRejectedPayment()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	if rendezvous.PublicKey().ToBase58() != base58.Encode(typedMessage.GetIntentId().Value) {
		return newMessageValidationErrorf("intent id must be %s", rendezvous.PublicKey().ToBase58())
	}

	return nil
}

func (h *ClientRejectedPaymentMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return false, 0 * time.Minute
}

func (h *ClientRejectedPaymentMessageHandler) OnSuccess(ctx context.Context) error {
	return nil
}

type CodeScannedMessageHandler struct {
}

func NewCodeScannedMessageHandler() MessageHandler {
	return &CodeScannedMessageHandler{}
}

func (h *CodeScannedMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetCodeScanned()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	return nil
}

func (h *CodeScannedMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return false, 0 * time.Minute
}

func (h *CodeScannedMessageHandler) OnSuccess(ctx context.Context) error {
	return nil
}

type RequestToLoginMessageHandler struct {
	data                 code_data.Provider
	rpcSignatureVerifier *auth.RPCSignatureVerifier
	domainVerifier       thirdparty.DomainVerifier

	recordAlreadyExists bool
	recordToSave        *paymentrequest.Record
}

func NewRequestToLoginMessageHandler(data code_data.Provider, rpcSignatureVerifier *auth.RPCSignatureVerifier, domainVerifier thirdparty.DomainVerifier) MessageHandler {
	return &RequestToLoginMessageHandler{
		data:                 data,
		rpcSignatureVerifier: rpcSignatureVerifier,
		domainVerifier:       domainVerifier,
	}
}

func (h *RequestToLoginMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetRequestToLogin()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	owner, err := common.NewAccountFromProto(typedMessage.Verifier)
	if err != nil {
		return err
	}

	signature := typedMessage.Signature
	typedMessage.Signature = nil
	if err := h.rpcSignatureVerifier.Authenticate(ctx, owner, typedMessage, signature); err != nil {
		return newMessageAuthenticationError("")
	}
	typedMessage.Signature = signature

	//
	// Part 1: Validate the intent doesn't exist
	//

	_, err = h.data.GetIntent(ctx, rendezvous.PublicKey().ToBase58())
	if err == nil {
		return newMessageValidationError("client submitted intent")
	} else if err != intent.ErrIntentNotFound {
		return err
	}

	//
	// Part 2: Validate the request metadata
	//

	asciiBaseDomain, err := thirdparty.GetAsciiBaseDomain(typedMessage.Domain.Value)
	if err != nil {
		return newMessageValidationErrorf("domain is invalid: %s", err.Error())
	}

	if !bytes.Equal(rendezvous.PublicKey().ToBytes(), typedMessage.RendezvousKey.Value) {
		return newMessageValidationError("rendezvous key mismatch")
	}

	existingRequestRecord, err := h.data.GetRequest(ctx, rendezvous.PublicKey().ToBase58())
	switch err {
	case nil:
		if existingRequestRecord.RequiresPayment() {
			return newMessageValidationError("original request requires payment")
		}

		if *existingRequestRecord.Domain != asciiBaseDomain {
			return newMessageValidationError("domain mismatches original request")
		}

		h.recordAlreadyExists = true
	case paymentrequest.ErrPaymentRequestNotFound:
	default:
		return err
	}

	//
	// Part 3: Domain validation
	//

	err = verifyThirdPartyDomain(ctx, h.domainVerifier, owner, typedMessage.Domain)
	if err != nil {
		return err
	}

	//
	// Part 4: Create the validated payment request DB record to store later,
	//         if it doesn't already exist
	//

	if !h.recordAlreadyExists {
		h.recordToSave = &paymentrequest.Record{
			Intent: rendezvous.PublicKey().ToBase58(),

			Domain:     &asciiBaseDomain,
			IsVerified: true,

			CreatedAt: time.Now(),
		}
	}

	return nil
}

func (h *RequestToLoginMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return false, 0 * time.Minute
}

func (h *RequestToLoginMessageHandler) OnSuccess(ctx context.Context) error {
	if h.recordAlreadyExists {
		return nil
	}
	return h.data.CreateRequest(ctx, h.recordToSave)
}

type ClientRejectedLoginMessageHandler struct {
}

func NewClientRejectedLoginMessageHandler() MessageHandler {
	return &ClientRejectedLoginMessageHandler{}
}

func (h *ClientRejectedLoginMessageHandler) Validate(ctx context.Context, rendezvous *common.Account, untypedMessage *messagingpb.Message) error {
	typedMessage := untypedMessage.GetClientRejectedLogin()
	if typedMessage == nil {
		return errors.New("invalid message type")
	}

	return nil
}

func (h *ClientRejectedLoginMessageHandler) RequiresActiveStream() (bool, time.Duration) {
	return false, 0 * time.Minute
}

func (h *ClientRejectedLoginMessageHandler) OnSuccess(ctx context.Context) error {
	return nil
}

func validateExchangeDataWithinMessage(ctx context.Context, data code_data.Provider, proto *transactionpb.ExchangeData) error {
	isValid, message, err := exchange_rate_util.ValidateClientExchangeData(ctx, data, proto)
	if err != nil {
		return err
	} else if !isValid {
		return newMessageValidationError(message)
	}
	return nil
}

func validateExternalKinTokenAccountWithinMessage(ctx context.Context, data code_data.Provider, tokenAccount *common.Account) error {
	isValid, message, err := common.ValidateExternalKinTokenAccount(ctx, data, tokenAccount)
	if err != nil {
		return err
	} else if !isValid {
		return newMessageValidationError(message)
	}
	return nil
}

func verifyThirdPartyDomain(ctx context.Context, verifier thirdparty.DomainVerifier, owner *common.Account, domain *commonpb.Domain) error {
	asciiBaseDomain, err := thirdparty.GetAsciiBaseDomain(domain.Value)
	if err != nil {
		return newMessageValidationErrorf("domain is invalid: %s", err.Error())
	}

	ownsDomain, err := verifier(ctx, owner, domain.Value)
	if err != nil {
		return newMessageAuthenticationErrorf("error veryfing domain ownership: %s", err.Error())
	} else if !ownsDomain {
		return newMessageAuthorizationErrorf("%s does not own domain %s", owner.PublicKey().ToBase58(), asciiBaseDomain)
	}

	return nil
}
