package webhook

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	messagingpb "github.com/code-payments/code-protobuf-api/generated/go/messaging/v1"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
	"github.com/code-payments/code-server/pkg/code/server/grpc/messaging"
)

const (
	metricsPackageName = "webhook"

	contentTypeHeaderName  = "Content-Type"
	contentTypeHeaderValue = "application/jwt"
)

// Execute executes the provided webhook. It does not manage the DB record's state.
func Execute(
	ctx context.Context,
	data code_data.Provider,
	messagingClient messaging.InternalMessageClient,
	record *webhook.Record,
	webhookTimeout time.Duration,
) error {
	tracer := metrics.TraceMethodCall(ctx, metricsPackageName, "Execute")
	defer tracer.End()

	err := func() error {
		//
		// Part 1: Basic validation checks
		//

		if record.State != webhook.StatePending {
			return errors.New("webhook is not in a pending state")
		}

		if record.NextAttemptAt == nil || record.NextAttemptAt.After(time.Now()) {
			return errors.New("webhook is not scheduled yet")
		}

		// Assumes all webhooks are tied to an intent
		rendezvousAccount, err := common.NewAccountFromPublicKeyString(record.WebhookId)
		if err != nil {
			return errors.Wrap(err, "webhook id is not a public key")
		}

		//
		// Part 2: Generate the JWT HTTP request body
		//

		jsonPayloadProvider, ok := jsonPayloadProviders[record.Type]
		if !ok {
			return errors.Errorf("%d webhook type not supported", record.Type)
		}

		kvs, err := jsonPayloadProvider(ctx, data, record)
		if err != nil {
			return errors.Wrap(err, "error getting webhook content")
		}

		signer := common.GetSubsidizer()
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims(kvs))
		requestBody, err := token.SignedString(ed25519.PrivateKey(signer.PrivateKey().ToBytes()))
		if err != nil {
			return errors.Wrap(err, "error signing jwt")
		}

		//
		// Part 3: Execute the HTTP POST
		//

		webhookReq, err := http.NewRequest(http.MethodPost, record.Url, strings.NewReader(requestBody))
		if err != nil {
			return errors.Wrap(err, "error creating http request")
		}
		webhookReq.Header.Set(contentTypeHeaderName, contentTypeHeaderValue)

		webhookCtx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
		webhookReq = webhookReq.WithContext(webhookCtx)
		defer cancel()

		resp, err := http.DefaultClient.Do(webhookReq)
		if err != nil {
			return errors.Wrap(err, "error executing http post request")
		} else if resp.StatusCode != http.StatusOK {
			return errors.Errorf("%d status code returned", resp.StatusCode)
		}

		//
		// Part 4: Notify the messaging stream on success
		//

		_, err = messagingClient.InternallyCreateMessage(ctx, rendezvousAccount, &messagingpb.Message{
			Kind: &messagingpb.Message_WebhookCalled{
				WebhookCalled: &messagingpb.WebhookCalled{
					Timestamp: timestamppb.Now(),
				},
			},
		})
		if err != nil {
			return errors.Wrap(err, "error creating notification message")
		}

		return nil
	}()

	if err != nil {
		tracer.OnError(err)
	}
	return err
}
