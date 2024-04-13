package request

import (
	"errors"
	"net/http"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/code-payments/code-server/pkg/code/common"
)

const (
	v1PathPrefix       = "/v1"
	v1CreateIntentPath = v1PathPrefix + "/createIntent"
	v1GetStatusPath    = v1PathPrefix + "/getStatus"
	v1GetUserIdPath    = v1PathPrefix + "/getUserId"

	contentTypeHeaderName      = "content-type"
	jsonContentTypeHeaderValue = "application/json"
)

type Server struct {
	log *logrus.Entry
	cc  *grpc.ClientConn
}

func NewRequestServer(cc *grpc.ClientConn) *Server {
	return &Server{
		log: logrus.StandardLogger().WithField("type", "request/server"),
		cc:  cc,
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

			model, err := newTrustlessRequestFromHttpContext(r)
			if err != nil {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(err)
			}

			err = s.createTrustlessRequest(ctx, model)
			if err != nil {
				log.WithError(err).Warn("failure creating request")
				statusCode, err := HandleGrpcErrorInWebContext(w, err)
				return statusCode, NewGenericApiFailureResponseBody(err)
			}

			return http.StatusOK, NewGenericApiSuccessResponseBody()
		}()

		w.Header().Set(contentTypeHeaderName, jsonContentTypeHeaderValue)
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(body.ToString())); err != nil {
			log.WithError(err).Info("failed to write body")
		}
	}
}

// todo: migrate to a POST variant
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

			res, err := s.getIntentStatus(ctx, intentId)
			if err != nil {
				log.WithError(err).Warn("failure getting intent status")
				statusCode, err := HandleGrpcErrorInWebContext(w, err)
				return statusCode, NewGenericApiFailureResponseBody(err)
			}

			respBody := NewGenericApiSuccessResponseBody()
			respBody["status"] = res.Status
			return http.StatusOK, respBody
		}()

		w.Header().Set(contentTypeHeaderName, jsonContentTypeHeaderValue)
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(body.ToString())); err != nil {
			log.WithError(err).Warn("failed to write body")
		}
	}
}

func (s *Server) getUserIdHandler(path string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := s.log.WithField("path", path)

		statusCode, body := func() (int, GenericApiResponseBody) {
			ctx := r.Context()

			if r.Method != http.MethodPost {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(errors.New("http post expected"))
			}

			protoReq, err := newGetLoggedInUserIdRequestFromHttpContext(r)
			if err != nil {
				return http.StatusBadRequest, NewGenericApiFailureResponseBody(err)
			}

			res, err := s.getUserId(ctx, protoReq)
			if err != nil {
				log.WithError(err).Warn("failure getting user")
				statusCode, err := HandleGrpcErrorInWebContext(w, err)
				return statusCode, NewGenericApiFailureResponseBody(err)
			}

			respBody := NewGenericApiSuccessResponseBody()
			if len(res.User) > 0 {
				respBody["user"] = res.User
			}
			return http.StatusOK, respBody
		}()

		w.Header().Set(contentTypeHeaderName, jsonContentTypeHeaderValue)
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(body.ToString())); err != nil {
			log.WithError(err).Warn("failed to write body")
		}
	}
}

func (s *Server) GetHandlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		v1CreateIntentPath: s.createIntentHandler(v1CreateIntentPath),
		v1GetStatusPath:    s.getStatusHandler(v1GetStatusPath),
		v1GetUserIdPath:    s.getUserIdHandler(v1GetUserIdPath),
	}
}
