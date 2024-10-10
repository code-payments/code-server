package async_vm

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/async"
	code_data "github.com/code-payments/code-server/pkg/code/data"
)

type service struct {
	log  *logrus.Entry
	data code_data.Provider
}

func New(data code_data.Provider) async.Service {
	return &service{
		log:  logrus.StandardLogger().WithField("service", "vm"),
		data: data,
	}
}

func (p *service) Start(ctx context.Context, interval time.Duration) error {
	go func() {
		err := p.storageInitWorker(ctx, interval)
		if err != nil && err != context.Canceled {
			p.log.WithError(err).Warn("storage init processing loop terminated unexpectedly")
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
