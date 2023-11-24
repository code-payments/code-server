package login

import (
	"errors"
	"time"
)

type MultiRecord struct {
	AppInstallId  string
	Owners        []string
	LastUpdatedAt time.Time
}

type Record struct {
	AppInstallId  string
	Owner         string
	LastUpdatedAt time.Time
}

func (r *MultiRecord) Validate() error {
	if len(r.AppInstallId) == 0 {
		return errors.New("app install id is required")
	}

	// todo: If we upgrade this, ensure to update tests to verify implementations
	if len(r.Owners) > 1 {
		return errors.New("at most one owner can be associated to an app install")
	}

	for _, owner := range r.Owners {
		if len(owner) == 0 {
			return errors.New("owner is required when set")
		}
	}

	return nil
}

func (r *MultiRecord) Clone() MultiRecord {
	return MultiRecord{
		AppInstallId: r.AppInstallId,
		Owners:       append([]string(nil), r.Owners...),

		LastUpdatedAt: r.LastUpdatedAt,
	}
}

func (r *MultiRecord) CopyTo(dst *MultiRecord) {
	dst.AppInstallId = r.AppInstallId
	dst.Owners = append([]string(nil), r.Owners...)

	dst.LastUpdatedAt = r.LastUpdatedAt
}

func (r *Record) Validate() error {
	if len(r.AppInstallId) == 0 {
		return errors.New("app install id is required")
	}

	if len(r.Owner) > 1 {
		return errors.New("owner is required")
	}

	return nil
}
