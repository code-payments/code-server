package transaction_v2

import (
	"context"
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/database/query"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/intent"
)

const maxHistoryPageSize = 100

func (s *transactionServer) GetPaymentHistory(ctx context.Context, req *transactionpb.GetPaymentHistoryRequest) (*transactionpb.GetPaymentHistoryResponse, error) {
	log := s.log.WithField("method", "GetPaymentHistory")
	log = client.InjectLoggingMetadata(ctx, log)

	owner, err := common.NewAccountFromProto(req.Owner)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return nil, status.Error(codes.Internal, "")
	}
	log = log.WithField("owner_account", owner.PublicKey().ToBase58())

	signature := req.Signature
	req.Signature = nil
	if err := s.auth.Authenticate(ctx, owner, req, signature); err != nil {
		return nil, err
	}

	// Get the number of results that should be returned
	var limit uint64
	if req.PageSize > 0 {
		limit = uint64(req.PageSize)
	} else {
		limit = maxHistoryPageSize
	}

	// Enforce proto limit
	if limit > maxHistoryPageSize {
		limit = maxHistoryPageSize
	}

	// Convert the proto ordering type to the internal type
	var direction query.Ordering
	if req.Direction == transactionpb.GetPaymentHistoryRequest_ASC {
		direction = query.Ascending
	} else {
		direction = query.Descending
	}

	// Convert the proto cursor type to our native uint64 cursor (ID)
	var cursor query.Cursor
	if req.Cursor != nil {
		cursor = req.Cursor.Value
	} else {
		cursor = query.ToCursor(0)
		if direction == query.Descending {
			cursor = query.ToCursor(math.MaxInt64 - 1)
		}
	}

	// Get all intents for the owner as both a source and destination
	intentRecords, err := s.data.GetAllIntentsByOwner(
		ctx,
		owner.PublicKey().ToBase58(),
		query.WithLimit(limit),
		query.WithDirection(direction),
		query.WithCursor(cursor),
	)
	if err == intent.ErrIntentNotFound {
		return &transactionpb.GetPaymentHistoryResponse{
			Result: transactionpb.GetPaymentHistoryResponse_NOT_FOUND,
		}, nil
	} else if err != nil {
		log.WithError(err).Warn("failure querying intent records")
		return nil, status.Error(codes.Internal, "")
	}

	var items []*transactionpb.PaymentHistoryItem
	for _, intentRecord := range intentRecords {
		var paymentType transactionpb.PaymentHistoryItem_PaymentType
		var isDeposit, isWithdrawal, isRemoteSend, isReturned, isAirdrop, isMicroPayment bool
		var exchangeData *transactionpb.ExchangeData
		var airdropType transactionpb.AirdropType

		// Extract payment details from intents, where applicable, into a view
		// that makes sense for a user.
		switch intentRecord.IntentType {
		case intent.SendPrivatePayment:
			paymentType = transactionpb.PaymentHistoryItem_SEND
			isRemoteSend = intentRecord.SendPrivatePaymentMetadata.IsRemoteSend
			isWithdrawal = intentRecord.SendPrivatePaymentMetadata.IsWithdrawal
			isDeposit = false
			isMicroPayment = intentRecord.SendPrivatePaymentMetadata.IsMicroPayment
			if intentRecord.InitiatorOwnerAccount != owner.PublicKey().ToBase58() {
				paymentType = transactionpb.PaymentHistoryItem_RECEIVE
				isWithdrawal = false
				isDeposit = intentRecord.SendPrivatePaymentMetadata.IsWithdrawal
			}

			// Funds moving within the same owner don't get populated when they're
			// used to support another payment flow that represents the history item
			// (eg. public withdrawals with private top ups)
			if isWithdrawal && intentRecord.InitiatorOwnerAccount == intentRecord.SendPrivatePaymentMetadata.DestinationOwnerAccount && !isMicroPayment {
				continue
			}

			// Don't show history items where the user voids the gift card.
			if isRemoteSend {
				claimedIntent, err := s.data.GetGiftCardClaimedIntent(ctx, intentRecord.SendPrivatePaymentMetadata.DestinationTokenAccount)
				if err == nil && claimedIntent.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard {
					continue
				} else if err != nil && err != intent.ErrIntentNotFound {
					log.WithError(err).Warn("failure getting gift card claimed intent")
					return nil, status.Error(codes.Internal, "")
				}
			}

			exchangeData = &transactionpb.ExchangeData{
				Currency:     string(intentRecord.SendPrivatePaymentMetadata.ExchangeCurrency),
				ExchangeRate: intentRecord.SendPrivatePaymentMetadata.ExchangeRate,
				NativeAmount: intentRecord.SendPrivatePaymentMetadata.NativeAmount,
				Quarks:       intentRecord.SendPrivatePaymentMetadata.Quantity,
			}
		case intent.SendPublicPayment:
			paymentType = transactionpb.PaymentHistoryItem_SEND
			isRemoteSend = false
			isWithdrawal = intentRecord.SendPublicPaymentMetadata.IsWithdrawal
			isDeposit = false
			isMicroPayment = false
			if intentRecord.InitiatorOwnerAccount != owner.PublicKey().ToBase58() {
				paymentType = transactionpb.PaymentHistoryItem_RECEIVE
				isWithdrawal = false
				isDeposit = intentRecord.SendPublicPaymentMetadata.IsWithdrawal
			}

			// Bonus airdrops only occur within Code->Code withdrawal flows
			if s.airdropper != nil {
				isAirdrop = (intentRecord.InitiatorOwnerAccount == s.airdropper.VaultOwner.PublicKey().ToBase58())
			}

			if isAirdrop {
				// todo: something less hacky
				if intentRecord.SendPublicPaymentMetadata.NativeAmount == 5.0 {
					airdropType = transactionpb.AirdropType_GIVE_FIRST_KIN
				} else if intentRecord.SendPublicPaymentMetadata.NativeAmount == 1.0 {
					airdropType = transactionpb.AirdropType_GET_FIRST_KIN
				}
			}

			exchangeData = &transactionpb.ExchangeData{
				Currency:     string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency),
				ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
				NativeAmount: intentRecord.SendPublicPaymentMetadata.NativeAmount,
				Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
			}
		case intent.ReceivePaymentsPrivately:
			// Other intents account for history items
			continue
		case intent.MigrateToPrivacy2022:
			// Don't show migrations for dust
			if intentRecord.MigrateToPrivacy2022Metadata.Quantity < kin.ToQuarks(1) {
				continue
			}

			paymentType = transactionpb.PaymentHistoryItem_RECEIVE
			isDeposit = true
			exchangeData = &transactionpb.ExchangeData{
				Currency:     string(currency.KIN),
				ExchangeRate: 1.0,
				NativeAmount: float64(intentRecord.MigrateToPrivacy2022Metadata.Quantity) / kin.QuarksPerKin,
				Quarks:       intentRecord.MigrateToPrivacy2022Metadata.Quantity,
			}
		case intent.ExternalDeposit:
			if intentRecord.ExternalDepositMetadata.DestinationOwnerAccount != owner.PublicKey().ToBase58() {
				continue
			}

			// Don't show deposits for dust
			if intentRecord.ExternalDepositMetadata.Quantity < kin.ToQuarks(1) {
				continue
			}

			paymentType = transactionpb.PaymentHistoryItem_RECEIVE
			isDeposit = true
			exchangeData = &transactionpb.ExchangeData{
				Currency:     string(currency.KIN),
				ExchangeRate: 1.0,
				NativeAmount: float64(intentRecord.ExternalDepositMetadata.Quantity) / kin.QuarksPerKin,
				Quarks:       intentRecord.ExternalDepositMetadata.Quantity,
			}
		case intent.ReceivePaymentsPublicly:
			// The intent to create the remote send gift card has no knowledge of the
			// destination owner since it's claimed at a later time, so the history item
			// must come from the intent receiving it.
			if !intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
				continue
			}

			// Don't show history items where the user voids the gift card.
			if intentRecord.ReceivePaymentsPubliclyMetadata.IsIssuerVoidingGiftCard {
				continue
			}

			paymentType = transactionpb.PaymentHistoryItem_RECEIVE
			isRemoteSend = intentRecord.ReceivePaymentsPubliclyMetadata.IsRemoteSend
			isReturned = intentRecord.ReceivePaymentsPubliclyMetadata.IsReturned
			isWithdrawal = false
			isDeposit = false

			exchangeData = &transactionpb.ExchangeData{
				Currency:     string(intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeCurrency),
				ExchangeRate: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalExchangeRate,
				NativeAmount: intentRecord.ReceivePaymentsPubliclyMetadata.OriginalNativeAmount,
				Quarks:       intentRecord.ReceivePaymentsPubliclyMetadata.Quantity,
			}
		default:
			continue
		}

		item := &transactionpb.PaymentHistoryItem{
			Cursor: &transactionpb.Cursor{
				Value: query.ToCursor(intentRecord.Id),
			},

			ExchangeData: exchangeData,

			PaymentType: paymentType,

			IsWithdraw:     isWithdrawal,
			IsDeposit:      isDeposit,
			IsRemoteSend:   isRemoteSend,
			IsReturned:     isReturned,
			IsAirdrop:      isAirdrop,
			IsMicroPayment: isMicroPayment,

			AirdropType: airdropType,

			Timestamp: timestamppb.New(intentRecord.CreatedAt),
		}

		items = append(items, item)
	}

	if len(items) == 0 {
		return &transactionpb.GetPaymentHistoryResponse{
			Result: transactionpb.GetPaymentHistoryResponse_NOT_FOUND,
		}, nil
	}

	return &transactionpb.GetPaymentHistoryResponse{
		Result: transactionpb.GetPaymentHistoryResponse_OK,
		Items:  items,
	}, nil
}
