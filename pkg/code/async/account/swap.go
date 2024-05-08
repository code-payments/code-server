package async_account

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"

	"github.com/code-payments/code-server/pkg/code/balance"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/usdc"
)

const (
	swapPushRetryThreshold = 5 * time.Minute
	minUsdcSwapBalance     = usdc.QuarksPerUsdc / 100 // $0.01 USD
)

func (p *service) swapRetryWorker(serviceCtx context.Context, interval time.Duration) error {
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__account_service__handle_swap_retry")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			records, err := p.data.GetPrioritizedAccountInfosRequiringSwapRetry(tracedCtx, swapPushRetryThreshold, 100)
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

					err := p.maybeTriggerAnotherSwap(tracedCtx, record)
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

// todo: Handle the case where there are no push tokens
func (p *service) maybeTriggerAnotherSwap(ctx context.Context, accountInfoRecord *account.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":        "maybeTriggerAnotherSwap",
		"owner_account": accountInfoRecord.OwnerAccount,
		"token_account": accountInfoRecord.TokenAccount,
	})

	if accountInfoRecord.AccountType != commonpb.AccountType_SWAP {
		log.Trace("skipping account that isn't a swap account")
		return errors.New("expected a swap account")
	}

	ownerAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.OwnerAccount)
	if err != nil {
		log.WithError(err).Warn("invalid owner account")
		return err
	}

	tokenAccount, err := common.NewAccountFromPublicKeyString(accountInfoRecord.TokenAccount)
	if err != nil {
		log.WithError(err).Warn("invalid token account")
		return err
	}

	balance, _, err := balance.CalculateFromBlockchain(ctx, p.data, tokenAccount)
	if err != nil {
		log.WithError(err).Warn("failure getting token account balance")
		return err
	}

	// Mark the swap as successful if the account is left with only dust
	if balance < minUsdcSwapBalance {
		err = markSwapRetrySuccessful(ctx, p.data, accountInfoRecord)
		if err != nil {
			log.WithError(err).Warn("failure updating account info record")
		}
		return err
	}

	err = push.SendTriggerSwapRpcPushNotification(
		ctx,
		p.data,
		p.pusher,
		ownerAccount,
	)
	if err != nil {
		log.WithError(err).Warn("failure sending push notification")
	}

	err = markSwapRetriedNow(ctx, p.data, accountInfoRecord)
	if err != nil {
		log.WithError(err).Warn("failure updating account info record")
	}
	return err
}

func markSwapRetrySuccessful(ctx context.Context, data code_data.Provider, record *account.Record) error {
	if !record.RequiresSwapRetry {
		return nil
	}

	record.RequiresSwapRetry = false
	return data.UpdateAccountInfo(ctx, record)
}

func markSwapRetriedNow(ctx context.Context, data code_data.Provider, record *account.Record) error {
	if !record.RequiresSwapRetry {
		return nil
	}

	record.LastSwapRetryAt = time.Now()
	return data.UpdateAccountInfo(ctx, record)
}
