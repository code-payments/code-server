package request

// todo: put all of this somewhere more common

import (
	"encoding/json"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	successJsonKey = "success"
	errorJsonKey   = "error"
)

type GenericApiResponseBody map[string]any

func NewGenericApiSuccessResponseBody() GenericApiResponseBody {
	return map[string]any{
		successJsonKey: true,
	}
}

func NewGenericApiFailureResponseBody(err error) GenericApiResponseBody {
	return map[string]any{
		successJsonKey: false,
		errorJsonKey:   err.Error(),
	}
}

func (b *GenericApiResponseBody) ToString() string {
	marshalled, _ := json.Marshal(b)
	return string(marshalled)
}

func HandleGrpcErrorInWebContext(w http.ResponseWriter, err error) (int, error) {
	if err == nil {
		return http.StatusOK, nil
	}

	statusErr, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError, errors.New("internal server error")
	}

	switch statusErr.Code() {
	case codes.OK:
		return http.StatusOK, nil
	case codes.InvalidArgument:
		return http.StatusBadRequest, err
	case codes.Unauthenticated:
		return http.StatusUnauthorized, errors.New("authentication failed")
	case codes.PermissionDenied:
		return http.StatusForbidden, errors.New("permission denied")
	case codes.Canceled, codes.DeadlineExceeded:
		return http.StatusRequestTimeout, errors.New("request timed out")
	default:
		return http.StatusInternalServerError, errors.New("internal server error")
	}
}
