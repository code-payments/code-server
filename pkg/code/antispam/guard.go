package antispam

import (
	"github.com/sirupsen/logrus"

	code_data "github.com/code-payments/code-server/pkg/code/data"
)

// Guard is an antispam guard that checks whether operations of interest are
// allowed to be performed.
//
// Note: Implementation assumes distributed locking has already occurred for
// all methods.
type Guard struct {
	log  *logrus.Entry
	data code_data.Provider
}

func NewGuard(
	data code_data.Provider,
) *Guard {
	return &Guard{
		log:  logrus.StandardLogger().WithField("type", "antispam/guard"),
		data: data,
	}
}
