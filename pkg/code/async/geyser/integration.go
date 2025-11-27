package async_geyser

import (
	"context"

	"github.com/code-payments/code-server/pkg/code/common"
)

// Integration allows for notifications based on events processed by Geyser
type Integration interface {
	OnDepositReceived(ctx context.Context, owner, mint *common.Account, currencyName string, usdMarketValue float64) error
}
