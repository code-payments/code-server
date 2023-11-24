package antispam

import (
	"context"
)

// This isn't generally very useful (at least for incentive spammers). Let's
// keep it clean for now.
func isIpBanned(ctx context.Context) bool {
	return false
}
