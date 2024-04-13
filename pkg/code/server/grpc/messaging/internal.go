package messaging

import (
	"context"
	"crypto/ed25519"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/messaging"
	"github.com/code-payments/code-server/pkg/code/data/rendezvous"
	"github.com/code-payments/code-server/pkg/grpc/headers"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	internalSignatureHeaderName = "code-signature"
)

// todo: Similar to the common push package, we should put message creation and
//       a proper client (ie. not tied to the server) in a common messaging package.

type InternalMessageClient interface {
	// InternallyCreateMessage creates and forwards a message on a stream
	// identified by the rendezvous key
	InternallyCreateMessage(ctx context.Context, rendezvousKey *common.Account, message *messagingpb.Message) (uuid.UUID, error)
}

// Note: Assumes messages are generated in a RPC server where the messaging
// service exists. This likely won't be a good assumption (eg. message generated
// in a worker), but is good enough to enable some initial use cases (eg. payment
// requests). This is mostly an optimization around not needing to create a gRPC
// client if the stream and message generation are on the same server.
func (s *server) InternallyCreateMessage(ctx context.Context, rendezvousKey *common.Account, message *messagingpb.Message) (uuid.UUID, error) {
	if message.Id != nil {
		return uuid.Nil, errors.New("message.id is generated in InternallyCreateMessage")
	}

	if message.SendMessageRequestSignature != nil {
		return uuid.Nil, errors.New("message.send_message_request_signature cannot be set")
	}

	// Required for messages created outside the context of an RPC call
	var err error
	if !headers.AreHeadersInitialized(ctx) {
		ctx, err = headers.ContextWithHeaders(ctx)
		if err != nil {
			return uuid.Nil, errors.Wrap(err, "error initializing headers")
		}
	}

	id := uuid.New()
	idBytes, _ := id.MarshalBinary()
	message.Id = &messagingpb.MessageId{
		Value: idBytes,
	}

	if err := message.Validate(); err != nil {
		return uuid.Nil, errors.Wrap(err, "message failed validation")
	}

	messageBytes, err := proto.Marshal(message)
	if err != nil {
		return uuid.Nil, errors.Wrap(err, "error marshalling message")
	}

	record := &messaging.Record{
		Account:   rendezvousKey.PublicKey().ToBase58(),
		MessageID: id,
		Message:   messageBytes,
	}

	// Save the message to the DB
	err = s.data.CreateMessage(ctx, record)
	if err != nil {
		return uuid.Nil, errors.Wrap(err, "error saving message to db")
	}

	// Best effort attempt to forward the message to the active stream
	attempts, err := retry.Retry(
		func() error {
			return s.internallyForwardMessage(ctx, &messagingpb.SendMessageRequest{
				RendezvousKey: &messagingpb.RendezvousKey{
					Value: rendezvousKey.ToProto().Value,
				},
				Message: message,
				Signature: &commonpb.Signature{
					// Needs to be set to pass validation, but won't be used. This
					// is only required for client-initiated messages. Rendezvous
					// private keys are typically hidden from server.
					//
					// todo: Different RPCs for public versus internal message sending.
					Value: make([]byte, 64),
				},
			})
		},
		retry.Limit(5),
		retry.Backoff(backoff.BinaryExponential(100*time.Millisecond), 500*time.Millisecond),
	)
	if err != nil {
		s.log.
			WithError(err).
			WithFields(logrus.Fields{
				"method":   "InternallyCreateMessage",
				"attempts": attempts,
			}).
			Warn("failed to forward message (best effort)")
	}

	return id, nil
}

func (s *server) internallyForwardMessage(ctx context.Context, req *messagingpb.SendMessageRequest) error {
	streamKey := base58.Encode(req.RendezvousKey.Value)

	log := s.log.WithFields(logrus.Fields{
		"method":         "internallyForwardMessage",
		"rendezvous_key": streamKey,
	})

	rendezvousRecord, err := s.data.GetRendezvous(ctx, streamKey)
	if err == nil {
		log := log.WithField("receiver_location", rendezvousRecord.Location)

		// We got lucky and the receiver's stream is on the same RPC server as
		// where the message is created. No forwarding between servers is required.
		// Note that we always use the rendezvous record as the source of truth
		// instead of checking for an active stream on this server. This server's
		// active stream may not be holding the lock, which can only be determined
		// by who set the location in the rendezvous record.
		if rendezvousRecord.Location == s.broadcastAddress {
			s.streamsMu.RLock()
			stream := s.streams[streamKey]
			s.streamsMu.RUnlock()

			if stream != nil {
				if err := stream.notify(req.Message, notifyTimeout); err != nil {
					log.WithError(err).Warnf("failed to notify session stream, closing streamer (stream=%p)", stream)
				}
			}

			return nil
		}

		// Old rendezvous record that likely wasn't cleaned up. Avoid forwarding,
		// since we expect a broken state.
		if time.Since(rendezvousRecord.LastUpdatedAt) > rendezvousRecordMaxAge {
			return nil
		}

		client, cleanup, err := getInternalMessagingClient(rendezvousRecord.Location)
		if err != nil {
			log.WithError(err).Warn("failure creating internal grpc messaging client")
			return err
		}
		defer func() {
			if err := cleanup(); err != nil {
				log.WithError(err).Warn("failed to cleanup internal messaging client")
			}
		}()

		reqBytes, err := proto.Marshal(req)
		if err != nil {
			log.WithError(err).Warn("failure marshalling request proto")
			return err
		}
		reqSignature := ed25519.Sign(common.GetSubsidizer().PrivateKey().ToBytes(), reqBytes)

		err = headers.SetASCIIHeader(ctx, internalSignatureHeaderName, base58.Encode(reqSignature))
		if err != nil {
			log.WithError(err).Warn("failure setting signature header")
			return err
		}

		log.Trace("forwarding message")

		// Any errors forwarding the message need to be propagated back to the client.
		// It'll be up to the client to attempt a retry with a new message. Duplicates
		// are ok with the current message stream use cases.
		resp, err := client.SendMessage(ctx, req)
		if err != nil {
			log.WithError(err).Warn("failure sending redirected request")
			return err
		} else if resp.Result != messagingpb.SendMessageResponse_OK {
			log.WithField("result", resp.Result).Warn("non-OK result sending redirected request")
			return err
		}
	} else if err != rendezvous.ErrNotFound {
		log.WithError(err).Warn("failure getting rendezvous record")
		return err
	}

	return nil
}

func (s *server) verifyForwardedSendMessageRequest(ctx context.Context, req *messagingpb.SendMessageRequest) (bool, error) {
	signature, _ := headers.GetASCIIHeaderByName(ctx, internalSignatureHeaderName)
	if len(signature) == 0 {
		return false, nil
	}

	signatureBytes, err := base58.Decode(signature)
	if err != nil {
		return false, err
	}

	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return false, err
	}

	return ed25519.Verify(common.GetSubsidizer().PublicKey().ToBytes(), reqBytes, signatureBytes), nil
}
