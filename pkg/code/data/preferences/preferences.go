package preferences

import (
	"time"

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
