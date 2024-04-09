package twitter

import (
	"errors"
	"time"

	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"
)

type Record struct {
	Id uint64

	Username      string
	Name          string
	ProfilePicUrl string
	VerifiedType  userpb.TwitterUser_VerifiedType
	FollowerCount uint32

	TipAddress string

	LastUpdatedAt time.Time
	CreatedAt     time.Time
}

func (r *Record) Validate() error {
	if len(r.Username) == 0 {
		return errors.New("username is required")
	}

	if len(r.Name) == 0 {
		return errors.New("name is required")
	}

	if len(r.ProfilePicUrl) == 0 {
		return errors.New("profile pic url is required")
	}

	if len(r.TipAddress) == 0 {
		return errors.New("tip address is required")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id: r.Id,

		Username:      r.Username,
		Name:          r.Name,
		ProfilePicUrl: r.ProfilePicUrl,
		VerifiedType:  r.VerifiedType,
		FollowerCount: r.FollowerCount,

		TipAddress: r.TipAddress,

		LastUpdatedAt: r.LastUpdatedAt,
		CreatedAt:     r.CreatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id

	dst.Username = r.Username
	dst.Name = r.Name
	dst.ProfilePicUrl = r.ProfilePicUrl
	dst.VerifiedType = r.VerifiedType
	dst.FollowerCount = r.FollowerCount

	dst.TipAddress = r.TipAddress

	dst.LastUpdatedAt = r.LastUpdatedAt
	dst.CreatedAt = r.CreatedAt
}
