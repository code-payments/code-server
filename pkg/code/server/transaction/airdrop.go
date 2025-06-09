package transaction_v2

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	currency_util "github.com/code-payments/code-server/pkg/code/currency"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/currency"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/grpc/client"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/solana/cvm"
)

type AirdropType uint8

const (
	AirdropTypeUnknown AirdropType = iota
	AirdropTypeOnboardingBonus
	AirdropTypeWelcomeBonus
)

var (
	ErrInvalidAirdropTarget          = errors.New("invalid airdrop target owner account")
	ErrInsufficientAirdropperBalance = errors.New("insufficient airdropper balance")
	ErrIneligibleForAirdrop          = errors.New("user isn't eligible for airdrop")
)

var (
	cachedAirdropStatus = cache.NewCache(10_000)
)

type AirdropIntegration interface {
	// GetWelcomeBonusAmount returns the amount that should be paid for the
	// welcome bonus. Return 0 amount if the airdrop should not be sent.
	GetWelcomeBonusAmount(ctx context.Context, owner *common.Account) (float64, currency_lib.Code, error)
}

type defaultAirdropIntegration struct{}

// NewDefaultAirdropIntegration retuns an AirdropIntegration that sends $1 USD
// to everyone
func NewDefaultAirdropIntegration() AirdropIntegration {
	return &defaultAirdropIntegration{}
}

func (i *defaultAirdropIntegration) GetWelcomeBonusAmount(ctx context.Context, owner *common.Account) (float64, currency_lib.Code, error) {
	return 1.0, currency_lib.USD, nil
}

func (s *transactionServer) Airdrop(ctx context.Context, req *transactionpb.AirdropRequest) (*transactionpb.AirdropResponse, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":       "Airdrop",
		"airdrop_type": req.AirdropType,
	})
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

	cacheKey := getAirdropCacheKey(owner, AirdropType(req.AirdropType))
	_, ok := cachedAirdropStatus.Retrieve(cacheKey)
	if ok {
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_ALREADY_CLAIMED,
		}, nil
	}

	ownerLock := s.ownerLocks.Get(owner.PublicKey().ToBytes())
	ownerLock.Lock()
	defer ownerLock.Unlock()

	intentId := GetAirdropIntentId(AirdropTypeWelcomeBonus, owner.PublicKey().ToBase58())
	_, err = s.data.GetIntent(ctx, intentId)
	if err == nil {
		cachedAirdropStatus.Insert(cacheKey, true, 1)
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_ALREADY_CLAIMED,
		}, nil
	} else if err != intent.ErrIntentNotFound {
		log.WithError(err).Warn("failure checking if airdrop was already claimed")
		return nil, status.Error(codes.Internal, "")
	}

	if !s.conf.enableAirdrops.Get(ctx) {
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_UNAVAILABLE,
		}, nil
	}

	if req.AirdropType != transactionpb.AirdropType_WELCOME_BONUS {
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_UNAVAILABLE,
		}, nil
	}

	if !s.conf.disableAntispamChecks.Get(ctx) {
		allow, err := s.antispamGuard.AllowWelcomeBonus(ctx, owner)
		if err != nil {
			log.WithError(err).Warn("failure performing antispam check")
			return nil, status.Error(codes.Internal, "")
		} else if !allow {
			return &transactionpb.AirdropResponse{
				Result: transactionpb.AirdropResponse_UNAVAILABLE,
			}, nil
		}
	}

	intentRecord, err := s.airdrop(ctx, intentId, owner, AirdropTypeWelcomeBonus)
	switch err {
	case nil:
	case ErrInsufficientAirdropperBalance, ErrInvalidAirdropTarget, ErrIneligibleForAirdrop:
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_UNAVAILABLE,
		}, nil
	default:
		log.WithError(err).Warn("failure airdropping account")
		return nil, status.Error(codes.Internal, "")
	}

	log.Debug("airdropped")

	cachedAirdropStatus.Insert(cacheKey, true, 1)

	return &transactionpb.AirdropResponse{
		Result: transactionpb.AirdropResponse_OK,
		ExchangeData: &transactionpb.ExchangeData{
			Currency:     string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency),
			ExchangeRate: intentRecord.SendPublicPaymentMetadata.ExchangeRate,
			NativeAmount: intentRecord.SendPublicPaymentMetadata.NativeAmount,
			Quarks:       intentRecord.SendPublicPaymentMetadata.Quantity,
		},
	}, nil
}

// Note: this function is idempotent with the given intent ID.
func (s *transactionServer) airdrop(ctx context.Context, intentId string, owner *common.Account, airdropType AirdropType) (*intent.Record, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":       "airdrop",
		"owner":        owner.PublicKey().ToBase58(),
		"intent":       intentId,
		"airdrop_type": airdropType.String(),
	})

	// Find the destination account, which will be the user's primary account
	primaryAccountInfoRecord, err := s.data.GetLatestAccountInfoByOwnerAddressAndType(ctx, owner.PublicKey().ToBase58(), commonpb.AccountType_PRIMARY)
	if err == account.ErrAccountInfoNotFound {
		log.Trace("owner cannot receive airdrop")
		return nil, ErrInvalidAirdropTarget
	} else if err != nil {
		log.WithError(err).Warn("failure getting primary account info record")
		return nil, err
	}
	destination, err := common.NewAccountFromPublicKeyString(primaryAccountInfoRecord.TokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid destination account")
		return nil, err
	}

	var nativeAmount float64
	var currencyCode currency_lib.Code
	switch airdropType {
	case AirdropTypeWelcomeBonus:
		nativeAmount, currencyCode, err = s.airdropIntegration.GetWelcomeBonusAmount(ctx, owner)
		if err != nil {
			log.WithError(err).Warn("failure getting welcome bonus amount from integration")
			return nil, err
		}
		if nativeAmount == 0 {
			log.Trace("integration did not allow welcome bonus")
			return nil, ErrInvalidAirdropTarget
		}
	default:
		return nil, errors.New("unhandled airdrop type")
	}
	log = log.WithFields(logrus.Fields{
		"native_amount": nativeAmount,
		"currency":      currencyCode,
	})

	exchangeRateTime := currency_util.GetLatestExchangeRateTime()

	usdRateRecord, err := s.data.GetExchangeRate(ctx, currency_lib.USD, exchangeRateTime)
	if err != nil {
		log.WithError(err).Warn("failure getting usd rate")
		return nil, err
	}

	var otherRateRecord *currency.ExchangeRateRecord
	switch currencyCode {
	case currency_lib.USD:
		otherRateRecord = usdRateRecord
	case common.CoreMintSymbol:
		otherRateRecord = &currency.ExchangeRateRecord{
			Time:   exchangeRateTime,
			Rate:   1.0,
			Symbol: string(common.CoreMintSymbol),
		}
	default:
		otherRateRecord, err = s.data.GetExchangeRate(ctx, currencyCode, exchangeRateTime)
		if err != nil {
			log.WithError(err).Warn("failure getting other rate")
			return nil, err
		}
	}

	coreMintAmount := nativeAmount / otherRateRecord.Rate
	quarkAmount := uint64(coreMintAmount * float64(common.CoreMintQuarksPerUnit))

	usdValue := usdRateRecord.Rate * coreMintAmount
	if usdValue > s.conf.maxAirdropUsdValue.Get(ctx) {
		log.Warn("airdrop exceeds max usd value")
		return nil, ErrIneligibleForAirdrop
	}

	var isAirdropperManuallyUnlocked bool
	s.airdropperLock.Lock()
	defer func() {
		if !isAirdropperManuallyUnlocked {
			s.airdropperLock.Unlock()
		}
	}()

	// Does the intent already exist?
	existingIntentRecord, err := s.data.GetIntent(ctx, intentId)
	if err == nil {
		return existingIntentRecord, nil
	} else if err != intent.ErrIntentNotFound {
		log.WithError(err).Warn("failure checking for existing airdrop intent")
		return nil, err
	}

	// Do a balance check. If there's insufficient balance, the feature is considered
	// to be over with until we get more funding.
	balance, err := balance.CalculateFromCache(ctx, s.data, s.airdropper.Vault)
	if err != nil {
		log.WithError(err).Warn("failure getting airdropper balance")
		return nil, err
	} else if balance < quarkAmount {
		log.WithFields(logrus.Fields{
			"balance":  balance,
			"required": quarkAmount,
		}).Warn("airdropper has insufficient balance")
		return nil, ErrInsufficientAirdropperBalance
	}

	// Make the intent to transfer the funds. We accomplish this by having the
	// sender setup and act like an internal Code account who always publicly
	// transfers the airdrop like a Code->Code payment.
	//
	// todo: This is an interesting concept we could consider expanding further.
	//       Instead of constructing and validating everything manually, we could
	//       have a proper client call SubmitIntent in a worker.

	selectedNonce, err := s.noncePool.GetNonce(ctx)
	if err != nil {
		log.WithError(err).Warn("failure selecting available nonce")
		return nil, err
	}
	defer func() {
		selectedNonce.ReleaseIfNotReserved(ctx)
	}()

	vixnHash := cvm.GetCompactTransferMessage(&cvm.GetCompactTransferMessageArgs{
		Source:       s.airdropper.Vault.PublicKey().ToBytes(),
		Destination:  destination.PublicKey().ToBytes(),
		Amount:       quarkAmount,
		NonceAddress: selectedNonce.Account.PublicKey().ToBytes(),
		NonceValue:   cvm.Hash(selectedNonce.Blockhash),
	})
	virtualSig := ed25519.Sign(s.airdropper.VaultOwner.PrivateKey().ToBytes(), vixnHash[:])

	intentRecord := &intent.Record{
		IntentId:   intentId,
		IntentType: intent.SendPublicPayment,

		SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
			DestinationOwnerAccount: owner.PublicKey().ToBase58(),
			DestinationTokenAccount: destination.PublicKey().ToBase58(),
			Quantity:                quarkAmount,

			ExchangeCurrency: currencyCode,
			ExchangeRate:     otherRateRecord.Rate,
			NativeAmount:     nativeAmount,
			UsdMarketValue:   usdValue,

			IsWithdrawal: false,
		},

		InitiatorOwnerAccount: s.airdropper.VaultOwner.PublicKey().ToBase58(),

		State: intent.StatePending,

		CreatedAt: time.Now(),
	}

	actionRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   0,
		ActionType: action.NoPrivacyTransfer,

		Source:      s.airdropper.Vault.PublicKey().ToBase58(),
		Destination: pointer.String(destination.PublicKey().ToBase58()),

		Quantity: pointer.Uint64(quarkAmount),

		State: action.StatePending,

		CreatedAt: time.Now(),
	}

	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   actionRecord.ActionId,
		ActionType: actionRecord.ActionType,

		FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,

		VirtualNonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
		VirtualBlockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),
		VirtualSignature: pointer.String(base58.Encode(virtualSig)),

		Source:      actionRecord.Source,
		Destination: pointer.StringCopy(actionRecord.Destination),

		DisableActiveScheduling: false,

		// IntentOrderingIndex unknown until intent record is saved
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		State: fulfillment.StateUnknown,

		CreatedAt: time.Now(),
	}

	err = s.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		err := s.data.SaveIntent(ctx, intentRecord)
		if err != nil {
			return err
		}

		err = s.data.PutAllActions(ctx, actionRecord)
		if err != nil {
			return err
		}

		fulfillmentRecord.IntentOrderingIndex = intentRecord.Id
		err = s.data.PutAllFulfillments(ctx, fulfillmentRecord)
		if err != nil {
			return err
		}

		err = selectedNonce.MarkReservedWithSignature(ctx, *fulfillmentRecord.VirtualSignature)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.WithError(err).Warn("failure creating airdrop intent")
		return nil, err
	}

	// Avoid blocking the airdropper for inessential processing
	s.airdropperLock.Unlock()
	isAirdropperManuallyUnlocked = true

	recordAirdropEvent(ctx, owner, airdropType)

	return intentRecord, nil
}

func (s *transactionServer) loadAirdropper(ctx context.Context) error {
	vaultRecord, err := s.data.GetKey(ctx, s.conf.airdropperOwnerPublicKey.Get(ctx))
	if err != nil {
		return err
	}

	ownerAccount, err := common.NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
	if err != nil {
		return err
	}

	timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.CoreMintAccount)
	if err != nil {
		return err
	}

	s.airdropper = timelockAccounts
	return nil
}

func GetAirdropIntentId(airdropType AirdropType, reference string) string {
	combined := fmt.Sprintf("airdrop-%d-%s", airdropType, reference)
	hashed := sha256.Sum256([]byte(combined))
	return base58.Encode(hashed[:])
}

func getAirdropCacheKey(owner *common.Account, airdropType AirdropType) string {
	return fmt.Sprintf("%s:%d\n", owner.PublicKey().ToBase58(), airdropType)
}

func (t AirdropType) String() string {
	switch t {
	case AirdropTypeUnknown:
		return "unknown"
	case AirdropTypeOnboardingBonus:
		return "onboarding_bonus"
	case AirdropTypeWelcomeBonus:
		return "welcome_bonus"
	}
	return "unknown"
}
