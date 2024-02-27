package preferences

import (
	"time"

	"github.com/pkg/errors"
	"golang.org/x/text/language"

	"github.com/code-payments/code-server/pkg/code/data/user"
)

var (
	defaultLocale = language.English
)

type Record struct {
	Id uint64

	DataContainerId user.DataContainerID

	Locale language.Tag

	LastUpdatedAt time.Time
}

func (r *Record) Validate() error {
	if err := r.DataContainerId.Validate(); err != nil {
		return errors.Wrap(err, "invalid data container id")
	}

	if r.Locale.String() == language.Und.String() {
		return errors.New("locale is undefined")
	}

	return nil
}

func (r *Record) Clone() Record {
	return Record{
		Id:              r.Id,
		DataContainerId: r.DataContainerId,
		Locale:          r.Locale,
		LastUpdatedAt:   r.LastUpdatedAt,
	}
}

func (r *Record) CopyTo(dst *Record) {
	dst.Id = r.Id
	dst.DataContainerId = r.DataContainerId
	dst.Locale = r.Locale
	dst.LastUpdatedAt = r.LastUpdatedAt
}

// GetDefaultPreferences returns the default set of user preferences
func GetDefaultPreferences(id *user.DataContainerID) *Record {
	return &Record{
		DataContainerId: *id,
		Locale:          defaultLocale,
		LastUpdatedAt:   time.Now(),
	}
}

// GetDefaultLocale returns the default locale setting
func GetDefaultLocale() language.Tag {
	return defaultLocale
}
