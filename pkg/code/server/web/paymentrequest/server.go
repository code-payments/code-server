package paymentrequest

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/freetype/truetype"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/code-payments/code-server/pkg/code/common"
)

const (
	v1PathPrefix       = "/v1"
	v1CreateIntentPath = v1PathPrefix + "/createIntent"
	v1GetStatusPath    = v1PathPrefix + "/getStatus"
	v1RequestPath      = v1PathPrefix + "/request"
	v1TestWebhookpath  = v1PathPrefix + "/testWebhook"

	intentHeaderName      = "x-code-intent"
	idempotencyHeaderName = "x-code-idempotency"

	contentTypeHeaderName      = "content-type"
	jsonContentTypeHeaderValue = "application/json"
	pngContentTypeHeaderValue  = "image/png"

	codePublicKey = "codeHy87wGD5oMRLG75qKqsSi1vWE3oxNyYmXo5F9YR"
)

var (
	pngEncoder png.Encoder
)

func init() {
	pngEncoder.CompressionLevel = png.BestSpeed
}

type Assets struct {
	RequestCardBackgroundLayer image.Image
	RequestCardAmountTextFont  *truetype.Font
}

type Server struct {
	log    *logrus.Entry
	cc     *grpc.ClientConn
	assets *Assets
}

func NewPaymentRequestServer(cc *grpc.ClientConn, assets *Assets) *Server {
	return &Server{
		log:    logrus.StandardLogger().WithField("type", "paymentrequest/server"),
		cc:     cc,
		assets: assets,
	}
}

func (s *Server) createIntentHandler(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := s.log.WithField("path", path)

		statusCode, body := func() (int, GenericApiResponseBody) {
			ctx := r.Context()

			if r.Method != http.MethodPost {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(errors.New("http post expected"))
			}

			model, err := newTrustlessPaymentRequestFromHttpContext(r)
			if err != nil {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(err)
			}

			err = s.createTrustlessPaymentRequest(ctx, model)
			if err != nil {
				log.WithError(err).Warn("failure creating payment request")
				statusCode, err := HandleGrpcErrorInWebContext(w, err)
				return statusCode, NewGenericApiFailureResponseBody(err)
			}

			return http.StatusOK, NewGenericApiSuccessResponseBody()
		}()

		w.Header().Set(contentTypeHeaderName, jsonContentTypeHeaderValue)
		w.WriteHeader(statusCode)
		w.Write([]byte(body.ToString()))
	}
}

func (s *Server) getStatusHandler(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := s.log.WithField("path", path)

		statusCode, body := func() (int, GenericApiResponseBody) {
			ctx := r.Context()

			if r.Method != http.MethodGet {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(errors.New("http get expected"))
			}

			intentIdQueryParam := r.URL.Query()["intent"]
			if len(intentIdQueryParam) < 1 {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(errors.New("intent query parameter missing"))
			}

			intentId, err := common.NewAccountFromPublicKeyString(intentIdQueryParam[0])
			if err != nil {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(errors.New("intent id is not a public key"))
			}
			log = log.WithField("intent", intentId.PublicKey().ToBase58())

			status, err := s.getIntentStatus(ctx, intentId)
			if err != nil {
				log.WithError(err).Warn("failure getting intent status")
				statusCode, err := HandleGrpcErrorInWebContext(w, err)
				return statusCode, NewGenericApiFailureResponseBody(err)
			}

			respBody := NewGenericApiSuccessResponseBody()
			respBody["status"] = status
			return http.StatusOK, respBody
		}()

		w.Header().Set(contentTypeHeaderName, jsonContentTypeHeaderValue)
		w.WriteHeader(statusCode)
		w.Write([]byte(body.ToString()))
	}
}

func (s *Server) requestHandler(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := s.log.WithField("path", path)

		ctx := r.Context()

		// Endpoint is disabled
		if true {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		model, err := newTrustedPaymentRequestFromHttpContext(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		intentId := model.GetPrivateRendezvousKey()
		idempotencyKey := model.GetIdempotencyKey()

		log = log.WithField("intent", intentId.PublicKey().ToBase58())

		err = s.createTrustedPaymentRequest(ctx, model)
		if err != nil {
			log.WithError(err).Warn("failure creating payment request")
			HandleGrpcErrorInWebContext(w, err)
			return
		}

		image, err := s.drawRequestCard(model)
		if err != nil {
			log.WithError(err).Warn("failure drawing request card")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		buf := new(bytes.Buffer)
		err = pngEncoder.Encode(buf, image)
		if err != nil {
			log.WithError(err).Warn("failure encoding png image")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set(contentTypeHeaderName, pngContentTypeHeaderValue)
		w.Header().Add(intentHeaderName, intentId.PublicKey().ToBase58())
		w.Header().Add(idempotencyHeaderName, base64.URLEncoding.EncodeToString(idempotencyKey[:]))
		w.Write(buf.Bytes())
	}
}

func (s *Server) testWebhookHandler(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := s.log.WithField("path", path)

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.WithError(err).Warn("failure reading http body")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		parsed, err := jwt.ParseWithClaims(string(body), jwt.MapClaims{}, func(_ *jwt.Token) (interface{}, error) {
			code, _ := common.NewAccountFromPublicKeyString(codePublicKey)
			return ed25519.PublicKey(code.PublicKey().ToBytes()), nil
		})
		if err != nil {
			log.WithError(err).Warn("failure parsing jwt")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		log.WithField("payload", parsed.Claims).Info("webhook received")
	}
}

func (s *Server) GetHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		v1CreateIntentPath: s.createIntentHandler(v1CreateIntentPath),
		v1GetStatusPath:    s.getStatusHandler(v1GetStatusPath),
		v1RequestPath:      s.requestHandler(v1RequestPath),
		v1TestWebhookpath:  s.testWebhookHandler(v1TestWebhookpath),
	}
}
