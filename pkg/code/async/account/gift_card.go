package async_account

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/retry"
)

const (
	giftCardAutoReturnIntentPrefix = "auto-return-gc-"
	giftCardExpiry                 = 24 * time.Hour
)

func (p *service) giftCardAutoReturnWorker(serviceCtx context.Context, interval time.Duration) error {
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__account_service__handle_gift_card_auto_return")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			records, err := p.data.GetPrioritizedAccountInfosRequiringAutoReturnCheck(tracedCtx, giftCardExpiry, 10)
			if err == account.ErrAccountInfoNotFound {
				return nil
			} else if err != nil {
				m.NoticeError(err)
				return err
			}

			var wg sync.WaitGroup
			for _, record := range records {
				wg.Add(1)

				go func(record *account.Record) {
					defer wg.Done()

					err := p.maybeInitiateGiftCardAutoReturn(tracedCtx, record)
					if err != nil {
						m.NoticeError(err)
					}
				}(record)
			}
			wg.Wait()

			return nil
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) maybeInitiateGiftCardAutoReturn(ctx context.Context, accountInfoRecord *account.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":  "maybeInitiateGiftCardAutoReturn",
		"account": accountInfoRecord.TokenAccount,
	})

	if accountInfoRecord.AccountType != commonpb.AccountType_REMOTE_SEND_GIFT_CARD {
		log.Trace("skipping account that isn't a gift card")
		return errors.New("expected a gift card account")
	}

	giftCardVaultAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid vault account")
		return err
	}

	_, err = p.data.GetGiftCardClaimedAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == nil {
		log.Trace("gift card is claimed and will be removed from worker queue")

		// Gift card is claimed, so take it out of the worker queue. The auto-return
		// action and fulfillment will be revoked in the fulfillment worker from generic
		// account closing flows.
		//
		// Note: It is possible the original issuer "claimed" the gift card. This is
		//       actually ideal because funds move in a more private manner through the
		//       temp incoming account versus the primary account.
		return markAutoReturnCheckComplete(ctx, p.data, accountInfoRecord)
	} else if err != action.ErrActionNotFound {
		return err
	}

	// Expiration window hasn't been met
	//
	// Note: Without distributed locks, we assume SubmitIntent uses expiry - delta
	//       to ensure race conditions aren't possible
	if time.Since(accountInfoRecord.CreatedAt) < giftCardExpiry {
		log.Trace("skipping gift card that hasn't hit the expiry window")
		return nil
	}

	log.Trace("initiating process to return gift card balance to issuer")

	// There's no action to claim the gift card and the expiry window has been met.
	// It's time to initiate the process of auto-returning the funds back to the
	// issuer.
	err = p.initiateProcessToAutoReturnGiftCard(ctx, giftCardVaultAccount)
	if err != nil {
		log.WithError(err).Warn("failure initiating process to return gift card balance to issuer")
		return err
	}
	return markAutoReturnCheckComplete(ctx, p.data, accountInfoRecord)
}

// Note: This is the first instance of handling a conditional action, and could be
// a good guide for similar actions in the future.
func (p *service) initiateProcessToAutoReturnGiftCard(ctx context.Context, giftCardVaultAccount *common.Account) error {
	giftCardIssuedIntent, err := p.data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	autoReturnAction, err := p.data.GetGiftCardAutoReturnAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err != nil {
		return err
	}

	autoReturnFulfillment, err := p.data.GetAllFulfillmentsByAction(ctx, autoReturnAction.Intent, autoReturnAction.ActionId)
	if err != nil {
		return err
	}

	// Add a payment history item to show the funds being returned back to the issuer
	err = insertAutoReturnPaymentHistoryItem(ctx, p.data, giftCardIssuedIntent)
	if err != nil {
		return err
	}

	// We need to update pre-sorting because close dormant fulfillments are always
	// inserted at the very last spot in the line.
	//
	// Must be the first thing to succeed! We cannot risk a deposit back into the
	// organizer to win a race in scheduling. By pre-sorting this to the end of
	// the gift card issued intent, we ensure the auto-return is blocked on any
	// fulfillments to setup the gift card. We'll also guarantee that subsequent
	// intents that utilize the primary account as a source of funds will be blocked
	// by the auto-return.
	err = updateCloseDormantAccountFulfillmentPreSorting(
		ctx,
		p.data,
		autoReturnFulfillment[0],
		giftCardIssuedIntent.Id,
		math.MaxInt32,
		0,
	)
	if err != nil {
		return err
	}

	// This will update the action's quantity, so balance changes are reflected. We
	// also unblock fulfillment scheduling by moving the action out of the unknown
	// state and into the pending state.
	err = scheduleCloseDormantAccountAction(
		ctx,
		p.data,
		autoReturnAction,
		giftCardIssuedIntent.SendPrivatePaymentMetadata.Quantity,
	)
	if err != nil {
		return err
	}

	// This will trigger the fulfillment worker to poll for the fulfillment. This
	// should be the very last DB update called.
	err = markFulfillmentAsActivelyScheduled(ctx, p.data, autoReturnFulfillment[0])
	if err != nil {
		return err
	}

	// Finally, update the user by best-effort sending them a push
	go push.SendGiftCardReturnedPushNotification(
		ctx,
		p.data,
		p.pusher,
		giftCardVaultAccount,
	)
	return nil
}

func markAutoReturnCheckComplete(ctx context.Context, data code_data.Provider, record *account.Record) error {
	if !record.RequiresAutoReturnCheck {
		return nil
	}

	record.RequiresAutoReturnCheck = false
	return data.UpdateAccountInfo(ctx, record)
}

// Note: Structured like a generic utility because it could very well evolve
// into that, but there's no reason to call this on anything else as of
// writing this comment.
func scheduleCloseDormantAccountAction(ctx context.Context, data code_data.Provider, actionRecord *action.Record, balance uint64) error {
	if actionRecord.ActionType != action.CloseDormantAccount {
		return errors.New("expected a close dormant account action")
	}

	if actionRecord.State == action.StatePending {
		return nil
	}

	if actionRecord.State != action.StateUnknown {
		return errors.New("expected action in unknown state")
	}

	actionRecord.State = action.StatePending
	actionRecord.Quantity = pointer.Uint64(balance)
	return data.UpdateAction(ctx, actionRecord)
}

// Note: Structured like a generic utility because it could very well evolve
// into that, but there's no reason to call this on anything else as of
// writing this comment.
func updateCloseDormantAccountFulfillmentPreSorting(
	ctx context.Context,
	data code_data.Provider,
	fulfillmentRecord *fulfillment.Record,
	intentOrderingIndex uint64,
	actionOrderingIndex uint32,
	fulfillmentOrderingIndex uint32,
) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.CloseDormantTimelockAccount {
		return errors.New("expected a close dormant timelock account fulfillment")
	}

	if fulfillmentRecord.IntentOrderingIndex == intentOrderingIndex &&
		fulfillmentRecord.ActionOrderingIndex == actionOrderingIndex &&
		fulfillmentRecord.FulfillmentOrderingIndex == fulfillmentOrderingIndex {
		return nil
	}

	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return errors.New("expected fulfillment in unknown state")
	}

	fulfillmentRecord.IntentOrderingIndex = intentOrderingIndex
	fulfillmentRecord.ActionOrderingIndex = actionOrderingIndex
	fulfillmentRecord.FulfillmentOrderingIndex = fulfillmentOrderingIndex
	return data.UpdateFulfillment(ctx, fulfillmentRecord)
}

func markFulfillmentAsActivelyScheduled(ctx context.Context, data code_data.Provider, fulfillmentRecord *fulfillment.Record) error {
	if fulfillmentRecord.Id == 0 {
		return errors.New("fulfillment id is zero")
	}

	if !fulfillmentRecord.DisableActiveScheduling {
		return nil
	}

	if fulfillmentRecord.State != fulfillment.StateUnknown {
		return errors.New("expected fulfillment in unknown state")
	}

	// Note: different than Save, since we don't have distributed locks
	return data.MarkFulfillmentAsActivelyScheduled(ctx, fulfillmentRecord.Id)
}

func insertAutoReturnPaymentHistoryItem(ctx context.Context, data code_data.Provider, giftCardIssuedIntent *intent.Record) error {
	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency.USD, time.Now())
	if err != nil {
		return err
	}

	// We need to insert a faked completed public receive intent so it can appear
	// as a return in the user's payment history. Think of it as a server-initiated
	// intent on behalf of the user based on pre-approved conditional actions.
	//
	// Deprecated in favour of chats (for history purposes)
	//
	// todo: Should we remap the CloseDormantAccount action and fulfillments, then
	//       tie the fulfillment/action state to the intent state? Just doing the
	//       easiest thing for now to get auto-return out the door.
	intentRecord := &intent.Record{
		IntentId:   getAutoReturnIntentId(giftCardIssuedIntent.IntentId),
		IntentType: intent.ReceivePaymentsPublicly,

		InitiatorOwnerAccount: giftCardIssuedIntent.InitiatorOwnerAccount,
		InitiatorPhoneNumber:  giftCardIssuedIntent.InitiatorPhoneNumber,

		ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
			Source:       giftCardIssuedIntent.SendPrivatePaymentMetadata.DestinationTokenAccount,
			Quantity:     giftCardIssuedIntent.SendPrivatePaymentMetadata.Quantity,
			IsRemoteSend: true,
			IsReturned:   true,

			OriginalExchangeCurrency: giftCardIssuedIntent.SendPrivatePaymentMetadata.ExchangeCurrency,
			OriginalExchangeRate:     giftCardIssuedIntent.SendPrivatePaymentMetadata.ExchangeRate,
			OriginalNativeAmount:     giftCardIssuedIntent.SendPrivatePaymentMetadata.NativeAmount,

			UsdMarketValue: usdExchangeRecord.Rate * float64(kin.FromQuarks(giftCardIssuedIntent.SendPrivatePaymentMetadata.Quantity)),
		},

		State: intent.StateConfirmed,

		CreatedAt: time.Now(),
	}
	err = data.SaveIntent(ctx, intentRecord)
	if err != nil {
		return err
	}

	return chat_util.SendCashTransactionsExchangeMessage(ctx, data, intentRecord)
}

// Must be unique, but consistent for idempotency, and ideally fit in a 32
// byte buffer.
func getAutoReturnIntentId(originalIntentId string) string {
	hashed := sha256.Sum256([]byte(giftCardAutoReturnIntentPrefix + originalIntentId))
	return base58.Encode(hashed[:])
}
