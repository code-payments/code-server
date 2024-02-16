package transaction_v2

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
)

func TestDeclareFiatOnrampPurchaseAttempt_HappyPath(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	nonce := uuid.New()

	currency := currency_lib.CAD
	amount := 123.456

	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_OK, phone.declareFiatOnRampPurchase(t, currency, amount, nonce))
	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_OK, phone.declareFiatOnRampPurchase(t, currency, amount, nonce))

	server.assertFiatOnrampPurchasedDetailsSaved(t, phone.parentAccount, currency, amount, nonce)
}

func TestDeclareFiatOnrampPurchaseAttempt_InvalidOwner(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	phone.reset(t)

	nonce := uuid.New()

	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_INVALID_OWNER, phone.declareFiatOnRampPurchase(t, currency_lib.USD, 50.00, nonce))

	server.assertFiatOnrampPurchasedDetailsNotSaved(t, nonce)
}

func TestDeclareFiatOnrampPurchaseAttempt_InvalidPurchaseAmount(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	nonce := uuid.New()

	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_UNSUPPORTED_CURRENCY, phone.declareFiatOnRampPurchase(t, "aaa", 10.00, nonce))
	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_UNSUPPORTED_CURRENCY, phone.declareFiatOnRampPurchase(t, currency_lib.KIN, 1.00, nonce))
	assert.Equal(t, transactionpb.DeclareFiatOnrampPurchaseAttemptResponse_AMOUNT_EXCEEDS_MAXIMUM, phone.declareFiatOnRampPurchase(t, currency_lib.USD, 250.01, nonce))

	server.assertFiatOnrampPurchasedDetailsNotSaved(t, nonce)
}
