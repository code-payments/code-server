package async_geyser

import (
	"context"

	"github.com/pkg/errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
)

// todo: needs to be reimagined for the VM

func findUnlockedTimelockV1Accounts(ctx context.Context, data code_data.Provider, daysFromToday uint8) ([]string, uint64, error) {
	return nil, 0, errors.New("not implemented")
}
