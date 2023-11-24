package push

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	push_data "github.com/code-payments/code-server/pkg/code/data/push"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

func getPushTokensForOwner(ctx context.Context, data code_data.Provider, owner *common.Account) ([]*push_data.Record, error) {
	verificationRecord, err := data.GetLatestPhoneVerificationForAccount(ctx, owner.PublicKey().ToBase58())
	if err != nil {
		return nil, errors.Wrap(err, "error getting latest phone verification record")
	}

	dataContainerRecord, err := data.GetUserDataContainerByPhone(ctx, owner.PublicKey().ToBase58(), verificationRecord.PhoneNumber)
	if err != nil {
		return nil, errors.Wrap(err, "error getting data container record")
	}

	pushTokenRecords, err := data.GetAllValidPushTokensdByDataContainer(ctx, dataContainerRecord.ID)
	if err == push_data.ErrTokenNotFound {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "error getting push token records")
	}
	return pushTokenRecords, nil
}

func onPushError(ctx context.Context, data code_data.Provider, pusher push_lib.Provider, pushTokenRecord *push_data.Record) (bool, error) {
	// On failure, verify token validity, and cleanup if necessary
	isValid, err := pusher.IsValidPushToken(ctx, pushTokenRecord.PushToken)
	if err == nil && !isValid {
		data.DeletePushToken(ctx, pushTokenRecord.PushToken)
	}
	return isValid, err
}

func getAmountArg(nativeAmount float64, currency currency_lib.Code) string {
	amountArg := fmt.Sprintf(
		"%d Kin",
		uint64(nativeAmount),
	)
	if currency != currency_lib.KIN {
		amountArg = fmt.Sprintf(
			"%s %.2f of Kin",
			strings.ToUpper(string(currency)),
			nativeAmount,
		)
	}
	return amountArg
}
