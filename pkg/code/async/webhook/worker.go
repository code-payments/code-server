package async_webhook

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	webhook_util "github.com/code-payments/code-server/pkg/code/webhook"
)

var (
	// General strategy:
	//  * Quick back-to-back attempts
	//  * Back off an order of time scale magnitude for next attempts
	//  * Bound the final delay to a day, where we'll decide to finally give up
	// The idea is to be as fast as possible, while also accounting for
	// third party server instability.
	attemptToDelay = map[uint8]time.Duration{
		1: 0,
		2: time.Second,
		3: time.Second,
		4: 15 * time.Second,
		5: time.Minute,
		6: 15 * time.Minute,
		7: time.Hour,
		8: 24 * time.Hour,

		// A final unused attempt, which is a hack to allow retryable transitions
		// of the record to the failed state. The delay should be reasonable enough
		// to ensure all other attempts have been executed.
		9: time.Minute,
	}
)

func (p *service) worker(serviceCtx context.Context, interval time.Duration) error {
	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			items, err := p.data.GetAllPendingWebhooksReadyToSend(serviceCtx, p.conf.workerBatchSize.Get(serviceCtx))
			if err == webhook.ErrNotFound {
				return nil
			} else if err != nil {
				return err
			}

			var wg sync.WaitGroup
			for _, item := range items {
				wg.Add(1)

				go func(record *webhook.Record) {
					nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
					m := nr.StartTransaction("async__webhook_service__handle_" + webhook.StatePending.String())
					defer m.End()
					tracedCtx := newrelic.NewContext(serviceCtx, m)

					err := p.handlePending(tracedCtx, record, &wg)
					if err != nil {
						m.NoticeError(err)
					}
				}(item)
			}
			wg.Wait()

			return nil
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) handlePending(ctx context.Context, record *webhook.Record, wg *sync.WaitGroup) error {
	if record.State != webhook.StatePending {
		return errors.New("record is not in pending state")
	}

	// Setup the next attempt synchronously, so it doesn't get picked up on
	// the next poll.
	shouldExecuteWebhook, err := p.setupNextAttempt(ctx, record)
	if err != nil {
		wg.Done()
		return err
	}
	wg.Done()

	if !shouldExecuteWebhook {
		return nil
	}

	// Handle the current attempt asynchronously, so slow third party servers
	// don't affect global webhook processing.
	return p.handleCurrentAttempt(ctx, record)
}

func (p *service) handleCurrentAttempt(ctx context.Context, record *webhook.Record) error {
	log := p.log.WithFields(logrus.Fields{
		"method":  "handleCurrentAttempt",
		"webhook": record.WebhookId,
	})

	executionErr := webhook_util.Execute(
		ctx,
		p.data,
		p.messagingClient,
		record,
		p.conf.webhookTimeout.Get(ctx),
	)
	callbackErr := p.onWebhookExecuted(ctx, record, executionErr == nil)

	if executionErr != nil {
		log.WithError(executionErr).Warn("failure executing webhook")
	}
	if callbackErr != nil {
		log.WithError(callbackErr).Warn("failure handling webhook result")
	}

	return errors.Join(executionErr, callbackErr)
}

func (p *service) setupNextAttempt(ctx context.Context, record *webhook.Record) (bool, error) {
	cloned := record.Clone()
	nextAttempt := cloned.Attempts + 2 // Because the current attempt hasn't been accounted for

	delay, ok := attemptToDelay[nextAttempt]
	if !ok {
		// All attempts exhausted. Mark it as failed.
		cloned.State = webhook.StateFailed
		cloned.NextAttemptAt = nil
		return false, p.updateWebhookRecord(ctx, &cloned)
	}

	record.Attempts += 1
	cloned.Attempts += 1
	cloned.NextAttemptAt = pointer.Time(time.Now().Add(delay))

	return true, p.updateWebhookRecord(ctx, &cloned)
}

func (p *service) onWebhookExecuted(ctx context.Context, record *webhook.Record, isSuccess bool) error {
	if record.State != webhook.StatePending {
		return nil
	}

	// Save success state immediately
	if isSuccess {
		p.metricsMu.Lock()
		p.successfulWebhooks += 1
		p.metricsMu.Unlock()

		record.State = webhook.StateConfirmed
		record.NextAttemptAt = nil
		return p.updateWebhookRecord(ctx, record)
	}

	p.metricsMu.Lock()
	p.failedWebhooks += 1
	p.metricsMu.Unlock()

	// Otherwise, save failure state only if we're on the last attempt
	if int(record.Attempts) < len(attemptToDelay) {
		return nil
	}

	record.State = webhook.StateFailed
	record.NextAttemptAt = nil
	return p.updateWebhookRecord(ctx, record)
}

func (p *service) updateWebhookRecord(ctx context.Context, record *webhook.Record) error {
	mu := p.webhookLocks.Get([]byte(record.WebhookId))
	mu.Lock()
	defer mu.Unlock()

	currentRecord, err := p.data.GetWebhook(ctx, record.WebhookId)
	if err != nil {
		return err
	}

	// Webhook already marked as confirmed, so new updates can be ignored
	if currentRecord.State == webhook.StateConfirmed {
		return nil
	}

	// Webhook already marked as failed, so any updates that aren't a confirmation can be ignored
	if currentRecord.State == webhook.StateFailed && record.State != webhook.StateConfirmed {
		return nil
	}

	// If both records are pending, we compare the attempt number and keep the largest
	if record.State == webhook.StatePending && record.Attempts <= currentRecord.Attempts {
		return nil
	}

	return p.data.UpdateWebhook(ctx, record)
}
