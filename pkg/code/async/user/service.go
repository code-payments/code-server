package async_user

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/sync"
	"github.com/code-payments/code-server/pkg/twitter"
)

type service struct {
	log           *logrus.Entry
	data          code_data.Provider
	twitterClient *twitter.Client
	userLocks     *sync.StripedLock
}

func New(twitterClient *twitter.Client, data code_data.Provider) async.Service {
	return &service{
		log:           logrus.StandardLogger().WithField("service", "user"),
		data:          data,
		twitterClient: twitterClient,
		userLocks:     sync.NewStripedLock(1024),
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	go func() {
		err := p.twitterRegistrationWorker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("twitter registration processing loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
