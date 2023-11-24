package push

import (
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/code/data/user"
)

type TokenType uint8

const (
	TokenTypeUnknown TokenType = iota
	TokenTypeFcmAndroid
	TokenTypeFcmApns
)

type Record struct {
	Id uint64

	DataContainerId user.DataContainerID

	PushToken string
	TokenType TokenType
	IsValid   bool

	AppInstallId *string

	CreatedAt time.Time
}

func (r *Record) Clone() *Record {
	return &Record{
		Id:              r.Id,
		DataContainerId: r.DataContainerId,
		PushToken:       r.PushToken,
		TokenType:       r.TokenType,
		IsValid:         r.IsValid,
		AppInstallId:    pointer.StringCopy(r.AppInstallId),
		CreatedAt:       r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.DataContainerId = r.DataContainerId
	dst.PushToken = r.PushToken
	dst.TokenType = r.TokenType
	dst.IsValid = r.IsValid
	dst.AppInstallId = pointer.StringCopy(r.AppInstallId)
	dst.CreatedAt = r.CreatedAt
}

func (r *Record) Validate() error {
	if err := r.DataContainerId.Validate(); err != nil {
		return errors.Wrap(err, "invalid data container id")
	}

	if len(r.PushToken) == 0 {
		return errors.New("push token is required")
	}

	if r.TokenType != TokenTypeFcmAndroid && r.TokenType != TokenTypeFcmApns {
		return errors.New("invalid token type")
	}

	if r.AppInstallId != nil && len(*r.AppInstallId) == 0 {
		return errors.New("app install id is required when set")
	}

	if r.CreatedAt.IsZero() {
		return errors.New("creation timestamp is required")
	}

	return nil
}
