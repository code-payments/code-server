package async_geyser

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	geyserpb "github.com/code-payments/code-server/pkg/code/async/geyser/api/gen"

	"github.com/code-payments/code-server/pkg/solana/token"
)

const (
	defaultStreamSubscriptionTimeout = time.Minute
)

var (
	ErrSubscriptionFallenBehind = errors.New("subscription stream fell behind")
	ErrTimeoutReceivingUpdate   = errors.New("timed out receiving update")
)

func newGeyserClient(ctx context.Context, endpoint string) (geyserpb.GeyserClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	client := geyserpb.NewGeyserClient(conn)

	// Unfortunately the RPCs we use no longer support hearbeats. We'll let each
	// individual subscriber determine what an appropriate timeout to receive a
	// message should be.
	/*
		heartbeatResp, err := client.GetHeartbeatInterval(ctx, &geyserpb.EmptyRequest{})
		if err != nil {
			return nil, 0, errors.Wrap(err, "error getting heartbeat interval")
		}

		heartbeatTimeout := time.Duration(2 * heartbeatResp.HeartbeatIntervalMs * uint64(time.Millisecond))
	*/

	return client, nil
}

func boundedProgramUpdateRecv(ctx context.Context, streamer geyserpb.Geyser_SubscribeProgramUpdatesClient, timeout time.Duration) (update *geyserpb.TimestampedAccountUpdate, err error) {
	done := make(chan struct{})
	go func() {
		update, err = streamer.Recv()
		close(done)
	}()

	select {
	case <-time.After(timeout):
		return nil, ErrTimeoutReceivingUpdate
	case <-done:
		return update, err
	}
}

func boundedSlotUpdateRecv(ctx context.Context, streamer geyserpb.Geyser_SubscribeSlotUpdatesClient, timeout time.Duration) (update *geyserpb.TimestampedSlotUpdate, err error) {
	done := make(chan struct{})
	go func() {
		update, err = streamer.Recv()
		close(done)
	}()

	select {
	case <-time.After(timeout):
		return nil, ErrTimeoutReceivingUpdate
	case <-done:
		return update, err
	}
}

func (p *service) subscribeToProgramUpdatesFromGeyser(ctx context.Context, endpoint string) error {
	log := p.log.WithField("method", "subscribeToProgramUpdatesFromGeyser")
	log.Debug("subscription started")

	defer func() {
		p.metricStatusLock.Lock()
		p.programUpdateSubscriptionStatus = false
		p.metricStatusLock.Unlock()

		log.Debug("subscription stopped")
	}()

	client, err := newGeyserClient(ctx, endpoint)
	if err != nil {
		return errors.Wrap(err, "error creating client")
	}

	streamer, err := client.SubscribeProgramUpdates(ctx, &geyserpb.SubscribeProgramsUpdatesRequest{
		Programs: [][]byte{token.ProgramKey},
	})
	if err != nil {
		return errors.Wrap(err, "error opening subscription stream")
	}

	var isSubscriptionActive bool
	for {
		update, err := boundedProgramUpdateRecv(ctx, streamer, defaultStreamSubscriptionTimeout)
		if err != nil {
			return errors.Wrap(err, "error recieving update")
		}

		// Observe at least one message before we can say we're up
		if !isSubscriptionActive {
			p.metricStatusLock.Lock()
			p.programUpdateSubscriptionStatus = true
			p.metricStatusLock.Unlock()

			isSubscriptionActive = true
		}

		messageAge := time.Since(update.Ts.AsTime())
		if messageAge > defaultStreamSubscriptionTimeout {
			log.WithField("message_age", messageAge).Warn(ErrSubscriptionFallenBehind.Error())
			return ErrSubscriptionFallenBehind
		}

		// Ignore startup updates. We only care about real-time updates due to
		// transactions.
		if update.AccountUpdate.IsStartup {
			continue
		}

		// Queue program updates for async processing. Most importantly, we need to
		// process messages from the gRPC subscription as fast as possible to avoid
		// backing up the Geyser plugin, which kills this subscription and we end up
		// missing updates.
		select {
		case p.programUpdatesChan <- update.AccountUpdate:
		default:
			log.Warn("dropping update because queue is full")
		}
	}
}

func (p *service) subscribeToSlotUpdatesFromGeyser(ctx context.Context, endpoint string) error {
	log := p.log.WithField("method", "subscribeToSlotUpdatesFromGeyser")
	log.Debug("subscription started")

	defer func() {
		p.metricStatusLock.Lock()
		p.slotUpdateSubscriptionStatus = false
		p.metricStatusLock.Unlock()

		log.Debug("subscription stopped")
	}()

	client, err := newGeyserClient(ctx, endpoint)
	if err != nil {
		return errors.Wrap(err, "error creating client")
	}

	streamer, err := client.SubscribeSlotUpdates(ctx, &geyserpb.SubscribeSlotUpdateRequest{})
	if err != nil {
		return errors.Wrap(err, "error opening subscription stream")
	}

	p.metricStatusLock.Lock()
	p.slotUpdateSubscriptionStatus = true
	p.metricStatusLock.Unlock()

	for {
		update, err := boundedSlotUpdateRecv(ctx, streamer, defaultStreamSubscriptionTimeout)
		if err != nil {
			return errors.Wrap(err, "error recieving update")
		}

		messageAge := time.Since(update.Ts.AsTime())
		if messageAge > defaultStreamSubscriptionTimeout {
			log.WithField("message_age", messageAge).Warn(ErrSubscriptionFallenBehind.Error())
			return ErrSubscriptionFallenBehind
		}

		if update.SlotUpdate.Status != geyserpb.SlotUpdateStatus_ROOTED {
			continue
		}

		p.metricStatusLock.Lock()
		if update.SlotUpdate.Slot > p.highestObservedRootedSlot {
			p.highestObservedRootedSlot = update.SlotUpdate.Slot
		}
		p.metricStatusLock.Unlock()
	}
}
