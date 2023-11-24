package async_geyser

import (
	"context"
	"errors"
	"time"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

var (
	errSignatureNotConfirmed = errors.New("signature is not confirmed")
	errSignatureNotFinalized = errors.New("signature is not finalized")
)

var waitForFinalizationRetryStrategies = []retry.Strategy{
	retry.NonRetriableErrors(context.Canceled),
	retry.Limit(30),
	retry.Backoff(backoff.Constant(3*time.Second), 3*time.Second),
}

var waitForConfirmationRetryStrategies = []retry.Strategy{
	retry.NonRetriableErrors(context.Canceled),
	retry.Limit(10),
	retry.Backoff(backoff.Constant(time.Second), time.Second),
}
