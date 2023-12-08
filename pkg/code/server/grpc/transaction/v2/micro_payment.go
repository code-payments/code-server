package transaction_v2

import (
	"context"
	"time"

	"github.com/pkg/errors"

	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/code-server/pkg/code/data/paymentrequest"
	"github.com/code-payments/code-server/pkg/code/push"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

func bestEffortMicroPaymentPostProcessingJob(
	data code_data.Provider,
	pusher push_lib.Provider,
	intentRecord *intent.Record,
	paymentRequestRecord *paymentrequest.Record,
) error {
	var isMicroPayment bool
	switch intentRecord.IntentType {
	case intent.SendPrivatePayment:
		isMicroPayment = intentRecord.SendPrivatePaymentMetadata.IsMicroPayment
	}
	if !isMicroPayment || paymentRequestRecord == nil {
		return errors.New("intent is not a micropayment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Send a push to the recipient of the micro payment
	push.SendMicroPaymentReceivedPushNotification(
		ctx,
		data,
		pusher,
		intentRecord,
		paymentRequestRecord,
	)

	return nil
}
