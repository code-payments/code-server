package async_account

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/action"
	"github.com/code-payments/code-server/pkg/code/data/fulfillment"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/retry"
)

const (
	giftCardAutoReturnIntentPrefix = "auto-return-gc-"
	GiftCardExpiry                 = 7 * 24 * time.Hour
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

			// todo: configurable batch size
			records, err := p.data.GetPrioritizedAccountInfosRequiringAutoReturnCheck(tracedCtx, GiftCardExpiry, 32)
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

	balanceLock, err := balance.GetOptimisticVersionLock(ctx, p.data, giftCardVaultAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting balance lock")
		return err
	}

	_, err = p.data.GetGiftCardClaimedAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
	if err == nil {
		log.Trace("gift card is claimed and will be removed from worker queue")

		// Cleanup anything related to gift card auto-return, since it cannot be scheduled
		err = InitiateProcessToCleanupGiftCardAutoReturn(ctx, p.data, giftCardVaultAccount)
		if err != nil {
			log.WithError(err).Warn("failure cleaning up auto-return action")
			return err
		}

		// Gift card is claimed, so take it out of the worker queue.
		return MarkAutoReturnCheckComplete(ctx, p.data, accountInfoRecord)
	} else if err != action.ErrActionNotFound {
		return err
	}

	// Expiration window hasn't been met
	//
	// Note: Without distributed locks, we assume SubmitIntent uses expiry - delta
	//       to ensure race conditions aren't possible
	if time.Since(accountInfoRecord.CreatedAt) < GiftCardExpiry {
		log.Trace("skipping gift card that hasn't hit the expiry window")
		return nil
	}

	log.Trace("initiating process to return gift card balance to issuer")

	// There's no action to claim the gift card and the expiry window has been met.
	// It's time to initiate the process of auto-returning the funds back to the
	// issuer.
	err = InitiateProcessToAutoReturnGiftCard(ctx, p.data, giftCardVaultAccount, false, balanceLock)
	if err != nil {
		log.WithError(err).Warn("failure initiating process to return gift card balance to issuer")
		return err
	}

	// Gift card is auto-returned, so take it out of the worker queue
	return MarkAutoReturnCheckComplete(ctx, p.data, accountInfoRecord)
}

// Note: This is the first instance of handling a conditional action, and could be
// a good guide for similar actions in the future.
//
// todo: This probably belongs somewhere more common
func InitiateProcessToAutoReturnGiftCard(ctx context.Context, data code_data.Provider, giftCardVaultAccount *common.Account, isVoidedByUser bool, balanceLock *balance.OptimisticVersionLock) error {
	return data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		giftCardIssuedIntent, err := data.GetOriginalGiftCardIssuedIntent(ctx, giftCardVaultAccount.PublicKey().ToBase58())
		if err != nil {
			return err
		}

		autoReturnAction, err := data.GetGiftCardAutoReturnAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
		if err != nil {
			return err
		}
		if autoReturnAction.State != action.StateUnknown {
			return nil
		}

		autoReturnFulfillment, err := data.GetAllFulfillmentsByAction(ctx, autoReturnAction.Intent, autoReturnAction.ActionId)
		if err != nil {
			return err
		}

		// Add a intent record to show the funds being returned back to the issuer
		err = insertAutoReturnIntentRecord(ctx, data, giftCardIssuedIntent, isVoidedByUser)
		if err != nil {
			return err
		}

		// We need to update pre-sorting because auto-return fulfillments are always
		// inserted at the very last spot in the line.
		//
		// Must be the first thing to succeed! By pre-sorting this to the end of
		// the gift card issued intent, we ensure the auto-return is blocked on any
		// fulfillments to setup the gift card. We'll also guarantee that subsequent
		// intents that utilize the primary account as a source of funds will be blocked
		// by the auto-return.
		err = updateAutoReturnFulfillmentPreSorting(
			ctx,
			data,
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
		err = scheduleAutoReturnAction(
			ctx,
			data,
			autoReturnAction,
			giftCardIssuedIntent.SendPublicPaymentMetadata.Quantity,
		)
		if err != nil {
			return err
		}

		// This will trigger the fulfillment worker to poll for the fulfillment. This
		// should be the very last DB update called.
		err = markFulfillmentAsActivelyScheduled(ctx, data, autoReturnFulfillment[0])
		if err != nil {
			return err
		}

		return balanceLock.OnCommit(ctx, data)
	})
}

// todo: This probably belongs somewhere more common
func InitiateProcessToCleanupGiftCardAutoReturn(ctx context.Context, data code_data.Provider, giftCardVaultAccount *common.Account) error {
	return data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
		autoReturnAction, err := data.GetGiftCardAutoReturnAction(ctx, giftCardVaultAccount.PublicKey().ToBase58())
		if err != nil {
			return err
		}

		autoReturnFulfillment, err := data.GetAllFulfillmentsByAction(ctx, autoReturnAction.Intent, autoReturnAction.ActionId)
		if err != nil {
			return err
		}

		err = markActionAsRevoked(ctx, data, autoReturnAction)
		if err != nil {
			return err
		}

		// The sequencer will handle state transition and any cleanup
		return markFulfillmentAsActivelyScheduled(ctx, data, autoReturnFulfillment[0])
	})
}

func MarkAutoReturnCheckComplete(ctx context.Context, data code_data.Provider, record *account.Record) error {
	if !record.RequiresAutoReturnCheck {
		return nil
	}

	record.RequiresAutoReturnCheck = false
	return data.UpdateAccountInfo(ctx, record)
}

// Note: Structured like a generic utility because it could very well evolve
// into that, but there's no reason to call this on anything else as of
// writing this comment.
func scheduleAutoReturnAction(ctx context.Context, data code_data.Provider, actionRecord *action.Record, balance uint64) error {
	if actionRecord.ActionType != action.NoPrivacyWithdraw {
		return errors.New("expected a no privacy withdraw action")
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
func updateAutoReturnFulfillmentPreSorting(
	ctx context.Context,
	data code_data.Provider,
	fulfillmentRecord *fulfillment.Record,
	intentOrderingIndex uint64,
	actionOrderingIndex uint32,
	fulfillmentOrderingIndex uint32,
) error {
	if fulfillmentRecord.FulfillmentType != fulfillment.NoPrivacyWithdraw {
		return errors.New("expected a no privacy withdraw fulfillment")
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

func insertAutoReturnIntentRecord(ctx context.Context, data code_data.Provider, giftCardIssuedIntent *intent.Record, isVoidedByUser bool) error {
	usdExchangeRecord, err := data.GetExchangeRate(ctx, currency.USD, time.Now())
	if err != nil {
		return err
	}

	// We need to insert a faked completed public receive intent so it can appear
	// as a return in the user's payment history. Think of it as a server-initiated
	// intent on behalf of the user based on pre-approved conditional actions.
	intentRecord := &intent.Record{
		IntentId:   getAutoReturnIntentId(giftCardIssuedIntent.IntentId),
		IntentType: intent.ReceivePaymentsPublicly,

		InitiatorOwnerAccount: giftCardIssuedIntent.InitiatorOwnerAccount,

		ReceivePaymentsPubliclyMetadata: &intent.ReceivePaymentsPubliclyMetadata{
			Source:   giftCardIssuedIntent.SendPublicPaymentMetadata.DestinationTokenAccount,
			Quantity: giftCardIssuedIntent.SendPublicPaymentMetadata.Quantity,

			IsRemoteSend:            true,
			IsIssuerVoidingGiftCard: isVoidedByUser,
			IsReturned:              !isVoidedByUser,

			OriginalExchangeCurrency: giftCardIssuedIntent.SendPublicPaymentMetadata.ExchangeCurrency,
			OriginalExchangeRate:     giftCardIssuedIntent.SendPublicPaymentMetadata.ExchangeRate,
			OriginalNativeAmount:     giftCardIssuedIntent.SendPublicPaymentMetadata.NativeAmount,

			UsdMarketValue: usdExchangeRecord.Rate * float64(giftCardIssuedIntent.SendPublicPaymentMetadata.Quantity) / float64(common.CoreMintQuarksPerUnit),
		},

		State: intent.StateConfirmed,

		CreatedAt: time.Now(),
	}
	return data.SaveIntent(ctx, intentRecord)
}

func markActionAsRevoked(ctx context.Context, data code_data.Provider, actionRecord *action.Record) error {
	if actionRecord.State == action.StateRevoked {
		return nil
	}

	if actionRecord.State != action.StateUnknown {
		return errors.New("expected fulfillment in unknown state")
	}

	actionRecord.State = action.StateRevoked

	return data.UpdateAction(ctx, actionRecord)
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

	fulfillmentRecord.DisableActiveScheduling = false
	return data.UpdateFulfillment(ctx, fulfillmentRecord)
}

// Must be unique, but consistent for idempotency, and ideally fit in a 32
// byte buffer.
func getAutoReturnIntentId(originalIntentId string) string {
	hashed := sha256.Sum256([]byte(giftCardAutoReturnIntentPrefix + originalIntentId))
	return base58.Encode(hashed[:])
}
