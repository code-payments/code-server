package transaction_v2

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/cache"
	"github.com/code-payments/code-server/pkg/code/common"
)

// This is a quick and dirty file to get an initial airdrop feature out
//
// Important Note: If this code is executed elsewhere (eg. a worker), we'll want to either
// use an actual client that hits SubmitIntent directly, or update this code to for an
// intenral server process (eg. by using the correct nonce pool to avoid race conditions
// without distributed locks).
//
// Important Note: We generally assumes 1 account per phone number for simplicity

type AirdropType uint8

const (
	AirdropTypeUnknown AirdropType = iota
	AirdropTypeGiveFirstKin
	AirdropTypeGetFirstKin
)

var (
	ErrInvalidAirdropTarget          = errors.New("invalid airdrop target owner account")
	ErrInsufficientAirdropperBalance = errors.New("insufficient airdropper balance")
	ErrIneligibleForAirdrop          = errors.New("user isn't eligible for airdrop")
)

var (
	cachedAirdropStatus        = cache.NewCache(10_000)
	cachedFirstReceivesByOwner = cache.NewCache(10_000)
)

/*

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

	isEligible, err := s.data.IsEligibleForAirdrop(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting airdrop eligibility for owner account")
		return nil, status.Error(codes.Internal, "")
	}
	if !isEligible {
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_UNAVAILABLE,
		}, nil
	}

	ownerLock := s.ownerLocks.Get(owner.PublicKey().ToBytes())
	ownerLock.Lock()
	defer ownerLock.Unlock()

	newIntentId := GetNewAirdropIntentId(AirdropTypeGetFirstKin, owner.PublicKey().ToBase58())
	oldIntentId := GetOldAirdropIntentId(AirdropTypeGetFirstKin, owner.PublicKey().ToBase58())
	for _, intentId := range []string{
		newIntentId,
		oldIntentId,
	} {
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
	}

	if !s.conf.enableAirdrops.Get(ctx) {
		return &transactionpb.AirdropResponse{
			Result: transactionpb.AirdropResponse_UNAVAILABLE,
		}, nil
	}

	if req.AirdropType != transactionpb.AirdropType_GET_FIRST_KIN {
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

	intentRecord, err := s.airdrop(ctx, newIntentId, owner, AirdropTypeGetFirstKin)
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

// airdrop gives Kin airdrops denominated in USD for performing certain
// actions in the Code app. This funciton is idempotent with the given
// intent ID.
func (s *transactionServer) airdrop(ctx context.Context, intentId string, owner *common.Account, airdropType AirdropType) (*intent.Record, error) {
	log := s.log.WithFields(logrus.Fields{
		"method":       "airdrop",
		"owner":        owner.PublicKey().ToBase58(),
		"intent":       intentId,
		"airdrop_type": airdropType.String(),
	})

	// Check whether the phone number allowed to receive an airdrop broadly
	verificationRecord, err := s.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting phone verification record")
		return nil, err
	}

	var usdValue float64
	switch airdropType {
	case AirdropTypeGiveFirstKin:
		usdValue = 5.0
	case AirdropTypeGetFirstKin:
		usdValue = 1.0
	default:
		return nil, errors.New("unhandled airdrop type")
	}

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

	// Calculate the amount of quarks to send
	usdRateRecord, err := s.data.GetExchangeRate(ctx, currency_lib.USD, exchange_rate_util.GetLatestExchangeRateTime())
	if err != nil {
		log.WithError(err).Warn("failure getting usd rate")
		return nil, err
	}
	quarks := kin.ToQuarks(uint64(usdValue / usdRateRecord.Rate))

	// Add an additional Kin, so we can always have enough to send the full fiat amount
	quarks += kin.ToQuarks(1)

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
	} else if balance < quarks {
		log.WithFields(logrus.Fields{
			"balance":  balance,
			"required": quarks,
		}).Warn("airdropper has insufficient balance")
		return nil, ErrInsufficientAirdropperBalance
	}

	// Make the intent to transfer the funds. We accomplish this by having the
	// sender setup and act like an internal Code account who always publicly
	// transfers the airdrop like a Code->Code withdrawal.
	//
	// todo: This is an interesting concept we could consider expanding further.
	//       Instead of constructing and validating everything manually, we could
	//       have a proper client call SubmitIntent in a worker.

	selectedNonce, err := transaction.SelectAvailableNonce(ctx, s.data, nonce.EnvironmentSolana, nonce.EnvironmentInstanceSolanaMainnet, nonce.PurposeClientTransaction)
	if err != nil {
		log.WithError(err).Warn("failure selecting available nonce")
		return nil, err
	}
	defer func() {
		selectedNonce.ReleaseIfNotReserved()
		selectedNonce.Unlock()
	}()

	txn, err := transaction.MakeTransferWithAuthorityTransaction(
		selectedNonce.Account,
		selectedNonce.Blockhash,
		s.airdropper,
		destination,
		quarks,
	)
	if err != nil {
		log.WithError(err).Warn("failure making solana transaction")
		return nil, err
	}

	err = txn.Sign(common.GetSubsidizer().PrivateKey().ToBytes(), s.airdropper.VaultOwner.PrivateKey().ToBytes())
	if err != nil {
		log.WithError(err).Warn("failure signing solana transaction")
		return nil, err
	}

	intentRecord := &intent.Record{
		IntentId:   intentId,
		IntentType: intent.SendPublicPayment,

		SendPublicPaymentMetadata: &intent.SendPublicPaymentMetadata{
			DestinationOwnerAccount: owner.PublicKey().ToBase58(),
			DestinationTokenAccount: destination.PublicKey().ToBase58(),
			Quantity:                quarks,

			ExchangeCurrency: currency_lib.USD,
			ExchangeRate:     usdRateRecord.Rate,
			NativeAmount:     usdValue,
			UsdMarketValue:   usdValue,

			IsWithdrawal: true,
		},

		InitiatorOwnerAccount: s.airdropper.VaultOwner.PublicKey().ToBase58(),

		State: intent.StateUnknown,

		CreatedAt: time.Now(),
	}

	actionRecord := &action.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   0,
		ActionType: action.NoPrivacyTransfer,

		Source:      s.airdropper.Vault.PublicKey().ToBase58(),
		Destination: pointer.String(destination.PublicKey().ToBase58()),

		Quantity: pointer.Uint64(quarks),

		State: action.StatePending,

		CreatedAt: time.Now(),
	}

	fulfillmentRecord := &fulfillment.Record{
		Intent:     intentRecord.IntentId,
		IntentType: intentRecord.IntentType,

		ActionId:   actionRecord.ActionId,
		ActionType: actionRecord.ActionType,

		FulfillmentType: fulfillment.NoPrivacyTransferWithAuthority,
		Data:            txn.Marshal(),
		Signature:       pointer.String(base58.Encode(txn.Signature())),

		Nonce:     pointer.String(selectedNonce.Account.PublicKey().ToBase58()),
		Blockhash: pointer.String(base58.Encode(selectedNonce.Blockhash[:])),

		Source:      actionRecord.Source,
		Destination: pointer.StringCopy(actionRecord.Destination),

		DisableActiveScheduling: false,

		// IntentOrderingIndex unknown until intent record is saved
		ActionOrderingIndex:      0,
		FulfillmentOrderingIndex: 0,

		State: fulfillment.StateUnknown,

		CreatedAt: time.Now(),
	}

	var eventType event.EventType
	switch airdropType {
	case AirdropTypeGetFirstKin:
		eventType = event.WelcomeBonusClaimed
	default:
		return nil, errors.Errorf("no event type defined for %s airdrop", airdropType.String())
	}

	eventRecord := &event.Record{
		EventId:   intentRecord.IntentId,
		EventType: eventType,

		SourceCodeAccount: owner.PublicKey().ToBase58(),
		SourceIdentity:    verificationRecord.PhoneNumber,

		SpamConfidence: 0,

		CreatedAt: time.Now(),
	}
	event_util.InjectClientDetails(ctx, s.maxmind, eventRecord, true)

	var chatMessage *chatpb.ChatMessage
	switch airdropType {
	case AirdropTypeGetFirstKin:
		chatMessage, err = chat_util.ToWelcomeBonusMessage(intentRecord)
		if err != nil {
			return nil, err
		}
	case AirdropTypeGiveFirstKin:
		chatMessage, err = chat_util.ToReferralBonusMessage(intentRecord)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("no chat message defined for %s airdrop", airdropType.String())
	}

	var canPushChatMessage bool
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

		err = selectedNonce.MarkReservedWithSignature(ctx, *fulfillmentRecord.Signature)
		if err != nil {
			return err
		}

		err = s.data.SaveEvent(ctx, eventRecord)
		if err != nil {
			return err
		}

		canPushChatMessage, err = chat_util.SendCodeTeamMessage(ctx, s.data, owner, chatMessage)
		if err != nil {
			return err
		}

		// Intent is pending only after everything's been saved.
		intentRecord.State = intent.StatePending
		return s.data.SaveIntent(ctx, intentRecord)
	})
	if err != nil {
		log.WithError(err).Warn("failure creating airdrop intent")
		return nil, err
	}

	// Avoid blocking the airdropper for inessential processing
	s.airdropperLock.Unlock()
	isAirdropperManuallyUnlocked = true

	if canPushChatMessage {
		// Best-effort send a push
		push_util.SendChatMessagePushNotification(
			ctx,
			s.data,
			s.pusher,
			chat_util.CodeTeamName,
			owner,
			chatMessage,
		)
	}

	recordAirdropEvent(ctx, owner, airdropType, usdValue)

	return intentRecord, nil
}

func (s *transactionServer) isFirstReceiveFromOtherCodeUser(ctx context.Context, intentToCheck string, owner *common.Account) (bool, error) {
	log := s.log.WithFields(logrus.Fields{
		"method": "isFirstReceiveFromOtherCodeUser",
		"intent": intentToCheck,
		"owner":  owner.PublicKey().ToBase58(),
	})

	cached, ok := cachedFirstReceivesByOwner.Retrieve(owner.PublicKey().ToBase58())
	if ok {
		return intentToCheck == cached.(string), nil
	}

	verificationRecord, err := s.data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		log.WithError(err).Warn("failure getting phone verification record")
		return false, err
	}
	log = log.WithField("phone", verificationRecord.PhoneNumber)

	// Staff are excluded since they can have multiple accounts per phone number,
	// and shouldn't be counted regardless.
	userIdentityRecord, err := s.data.GetUserByPhoneView(ctx, verificationRecord.PhoneNumber)
	if err != nil {
		log.WithError(err).Warn("failure getting user identity record")
		return false, err
	} else if userIdentityRecord.IsStaffUser {
		return false, nil
	}

	// Take a small view of the user's initial intent history from the very beginning
	history, err := s.data.GetAllIntentsByOwner(
		ctx,
		owner.PublicKey().ToBase58(),
		query.WithLimit(100),
		query.WithDirection(query.Ascending),
	)
	if err != nil {
		log.WithError(err).Warn("failure loading owner history")
		return false, err
	}

	var firstReceive *intent.Record
	createdGiftCards := make(map[string]struct{})
	for _, historyItem := range history {
		switch historyItem.IntentType {
		case intent.SendPrivatePayment, intent.SendPublicPayment:
			// The owner iniated the send
			if historyItem.InitiatorOwnerAccount == owner.PublicKey().ToBase58() {
				// Keep track of any gift cards the user created for future gift card receives
				if historyItem.IntentType == intent.SendPrivatePayment && historyItem.SendPrivatePaymentMetadata.IsRemoteSend {
					createdGiftCards[historyItem.SendPrivatePaymentMetadata.DestinationTokenAccount] = struct{}{}
				}

				continue
			}

			// The airdropper initiated the send
			if historyItem.InitiatorOwnerAccount == s.airdropper.VaultOwner.PublicKey().ToBase58() {
				continue
			}

			firstReceive = historyItem
		case intent.ReceivePaymentsPublicly:
			// Not receiving a gift card
			if !historyItem.ReceivePaymentsPubliclyMetadata.IsRemoteSend {
				continue
			}

			_, ok := createdGiftCards[historyItem.ReceivePaymentsPubliclyMetadata.Source]
			if !ok {
				// User didn't create this gift card
				firstReceive = historyItem
			}
		case intent.MigrateToPrivacy2022:
			// User received their first Kin at some point with pre-privacy account
			if historyItem.MigrateToPrivacy2022Metadata.Quantity > 0 {
				firstReceive = historyItem
			}
		}

		if firstReceive != nil {
			break
		}
	}

	// Highly unlikely to happen. At this point we can say the user onboarded
	// themself sufficiently anways.
	if firstReceive == nil {
		return false, nil
	}

	cachedFirstReceivesByOwner.Insert(owner.PublicKey().ToBase58(), firstReceive.IntentId, 1)
	return firstReceive.IntentId == intentToCheck, nil
}

func GetOldAirdropIntentId(airdropType AirdropType, reference string) string {
	return fmt.Sprintf("airdrop-%d-%s", airdropType, reference)
}

// Consistent intent ID that maps to a 32 byte buffer
func GetNewAirdropIntentId(airdropType AirdropType, reference string) string {
	old := GetOldAirdropIntentId(airdropType, reference)
	hashed := sha256.Sum256([]byte(old))
	return base58.Encode(hashed[:])
}

func getAirdropCacheKey(owner *common.Account, airdropType AirdropType) string {
	return fmt.Sprintf("%s:%d\n", owner.PublicKey().ToBase58(), airdropType)
}

*/

func (s *transactionServer) mustLoadAirdropper(ctx context.Context) {
	log := s.log.WithFields(logrus.Fields{
		"method": "mustLoadAirdropper",
		"key":    s.conf.airdropperOwnerPublicKey.Get(ctx),
	})

	err := func() error {
		vaultRecord, err := s.data.GetKey(ctx, s.conf.airdropperOwnerPublicKey.Get(ctx))
		if err != nil {
			return err
		}

		ownerAccount, err := common.NewAccountFromPrivateKeyString(vaultRecord.PrivateKey)
		if err != nil {
			return err
		}

		timelockAccounts, err := ownerAccount.GetTimelockAccounts(common.CodeVmAccount, common.KinMintAccount)
		if err != nil {
			return err
		}

		s.airdropper = timelockAccounts
		return nil
	}()
	if err != nil {
		log.WithError(err).Fatal("failure loading account")
	}
}

func (t AirdropType) String() string {
	switch t {
	case AirdropTypeUnknown:
		return "unknown"
	case AirdropTypeGiveFirstKin:
		return "give_first_kin"
	case AirdropTypeGetFirstKin:
		return "get_first_kin"
	}
	return "unknown"
}
