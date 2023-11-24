package transaction_v2

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/limit"
)

func TestGetLimits_SendLimits_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 10000)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)

	usdLimits := limit.SendLimits[currency_lib.Code(currency_lib.USD)]
	usdRate, err := server.data.GetExchangeRate(server.ctx, currency_lib.USD, time.Now())
	require.NoError(t, err)

	var usdSent float64
	for {
		if usdSent > usdLimits.Daily-usdLimits.PerTransaction {
			break
		}

		limitsByCurrency := sendingPhone.getSendLimits(t)
		assert.Len(t, limitsByCurrency, 3)

		actual := limitsByCurrency[string(currency_lib.USD)]
		assert.EqualValues(t, usdLimits.PerTransaction, actual.NextTransaction)

		actual = limitsByCurrency[string(currency_lib.KIN)]
		assert.EqualValues(t, usdLimits.PerTransaction/usdRate.Rate, actual.NextTransaction)

		sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)

		usdSent += 123 * usdRate.Rate
	}

	limitsByCurrency := sendingPhone.getSendLimits(t)
	assert.Len(t, limitsByCurrency, 3)

	actual := limitsByCurrency[string(currency_lib.USD)]
	assert.EqualValues(t, usdLimits.Daily-usdSent, actual.NextTransaction)

	actual = limitsByCurrency[string(currency_lib.KIN)]
	assert.EqualValues(t, (usdLimits.Daily-usdSent)/usdRate.Rate, actual.NextTransaction)

	for {
		if usdSent > usdLimits.Daily {
			break
		}

		sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
		receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)

		usdSent += 42 * usdRate.Rate
	}

	limitsByCurrency = sendingPhone.getSendLimits(t)
	assert.Len(t, limitsByCurrency, 3)

	actual = limitsByCurrency[string(currency_lib.USD)]
	assert.EqualValues(t, 0, actual.NextTransaction)

	actual = limitsByCurrency[string(currency_lib.KIN)]
	assert.EqualValues(t, 0, actual.NextTransaction)
}

func TestGetLimits_DepositLimits_HappyPath_DailyConsumed(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	// Resetting phone to a state where there are only balances in the primary account
	phone.reset(t)
	server.phoneVerifyUser(t, phone)
	server.fundAccount(t, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), kin.ToQuarks(10_000))

	limit.MaxDailyDepositUsdAmount = 200 // Needs to be < 2x maxPerDepositUsdAmount so we don't trigger total balance checks
	limit.MaxPerDepositUsdAmount = 150

	usdRate, err := server.data.GetExchangeRate(server.ctx, currency_lib.USD, time.Now())
	require.NoError(t, err)

	maxPerDepositKinAmount := uint64(limit.MaxPerDepositUsdAmount / usdRate.Rate)

	server.generateAvailableNonces(t, 10000)

	phone.openAccounts(t).requireSuccess(t)

	depositLimits := phone.getDepositLimit(t)
	assert.Equal(t, kin.ToQuarks(maxPerDepositKinAmount), depositLimits.MaxQuarks)

	var usdSent float64
	for {
		if usdSent > limit.MaxDailyDepositUsdAmount-limit.MaxPerDepositUsdAmount {
			break
		}

		depositLimits := phone.getDepositLimit(t)
		assert.Equal(t, kin.ToQuarks(maxPerDepositKinAmount), depositLimits.MaxQuarks)

		phone.deposit777KinIntoOrganizer(t).requireSuccess(t)

		usdSent += 777 * usdRate.Rate
	}

	depositLimits = phone.getDepositLimit(t)
	assert.EqualValues(t, kin.ToQuarks(uint64((limit.MaxDailyDepositUsdAmount-usdSent)/usdRate.Rate)), depositLimits.MaxQuarks)
}

func TestGetLimits_DepositLimits_HappyPath_ExistingPrivateBalance(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	// Resetting phone to a state where there are only balances in the primary account
	phone.reset(t)
	server.phoneVerifyUser(t, phone)
	server.fundAccount(t, phone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), kin.ToQuarks(10_000))

	limit.MaxDailyDepositUsdAmount = 300
	limit.MaxPerDepositUsdAmount = 150

	usdRate, err := server.data.GetExchangeRate(server.ctx, currency_lib.USD, time.Now())
	require.NoError(t, err)

	maxPerDepositKinAmount := uint64(limit.MaxPerDepositUsdAmount / usdRate.Rate)

	server.generateAvailableNonces(t, 10000)

	phone.openAccounts(t).requireSuccess(t)

	depositLimits := phone.getDepositLimit(t)
	assert.Equal(t, kin.ToQuarks(maxPerDepositKinAmount), depositLimits.MaxQuarks)

	server.fundAccount(t, phone.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0), kin.ToQuarks(maxPerDepositKinAmount)/2)

	depositLimits = phone.getDepositLimit(t)
	assert.Equal(t, kin.ToQuarks(maxPerDepositKinAmount), depositLimits.MaxQuarks)

	server.fundAccount(t, phone.getTimelockVault(t, commonpb.AccountType_BUCKET_1_KIN, 0), kin.ToQuarks(maxPerDepositKinAmount)/2)

	depositLimits = phone.getDepositLimit(t)
	assert.EqualValues(t, 0, depositLimits.MaxQuarks)
}

func TestGetLimits_MicroPaymentLimits_HappyPath(t *testing.T) {
	server, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	server.generateAvailableNonces(t, 10000)

	phone.openAccounts(t).requireSuccess(t)

	actualMicroPaymentLimits := phone.getMicroPaymentLimits(t)
	assert.Len(t, actualMicroPaymentLimits, len(limit.MicroPaymentLimits))
	for currency, actualLimits := range actualMicroPaymentLimits {
		expectedLimit, ok := limit.MicroPaymentLimits[currency_lib.Code(currency)]
		require.True(t, ok)
		assert.EqualValues(t, expectedLimit.Max, actualLimits.MaxPerTransaction)
		assert.EqualValues(t, expectedLimit.Min, actualLimits.MinPerTransaction)
	}
}

func TestGetLimits_UnauthenticateddAccess(t *testing.T) {
	_, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	maliciousAccount := testutil.NewRandomAccount(t)

	req := &transactionpb.GetLimitsRequest{
		Owner:         phone.parentAccount.ToProto(),
		ConsumedSince: timestamppb.New(time.Now().Add(-24 * time.Hour)),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	_, err = phone.client.GetLimits(phone.ctx, req)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}
