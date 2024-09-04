package transaction_v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/common"
	chat_v1 "github.com/code-payments/code-server/pkg/code/data/chat/v1"
	currency_lib "github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/kin"
	timelock_token_v1 "github.com/code-payments/code-server/pkg/solana/timelock/v1"
	"github.com/code-payments/code-server/pkg/testutil"
)

func TestPaymentHistory_HappyPath(t *testing.T) {
	server, sendingPhone, receivingPhone, cleanup := setupTestEnv(t, &testOverrides{
		enableAirdrops: true,
	})
	defer cleanup()

	merchantDomain := "example.com"
	twitterUsername := "tipthisuser"

	server.generateAvailableNonces(t, 1000)
	server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))
	server.simulateTwitterRegistration(t, twitterUsername, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0))

	amountForPrivacyMigration := kin.ToQuarks(23)
	legacyTimelockVault, err := sendingPhone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy, common.KinMintAccount)
	require.NoError(t, err)
	server.fundAccount(t, legacyTimelockVault, amountForPrivacyMigration)

	sendingPhone.openAccounts(t).requireSuccess(t)
	receivingPhone.openAccounts(t).requireSuccess(t)
	sendingPhone.establishRelationshipWithMerchant(t, merchantDomain).requireSuccess(t)
	receivingPhone.establishRelationshipWithMerchant(t, merchantDomain).requireSuccess(t)

	server.fundAccount(t, getTimelockVault(t, sendingPhone.getAuthorityForRelationshipAccount(t, merchantDomain)), kin.ToQuarks(1_000_000))

	// [Cash Transactions] sendingPhone DEPOSITED 23 Kin
	sendingPhone.migrateToPrivacy2022(t, amountForPrivacyMigration).requireSuccess(t)

	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)

	// [Cash Transactions] sendingPhone   GAVE      $4.20 USD of Kin
	// [Cash Transactions] receivingPhone RECEIVED  $4.20 USD of Kin
	sendingPhone.send42KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.receive42KinFromCodeUser(t).requireSuccess(t)

	// [Cash Transactions] sendingPhone   WITHDREW  $77.70 USD of Kin
	// [Cash Transactions] receivingPhone DEPOSITED $77.70 USD of Kin
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)

	// [Cash Transactions] sendingPhone WITHDREW 123 Kin
	sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)

	// [Cash Transactions] receivingPhone DEPOSITED 10,000,000,000 Kin
	server.simulateExternalDepositHistoryItem(t, receivingPhone.parentAccount, receivingPhone.getTimelockVault(t, commonpb.AccountType_PRIMARY, 0), kin.ToQuarks(10_000_000_000))
	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)

	// [Cash Transactions] sendingPhone   WITHDREW  $77.70 USD of Kin
	// [Cash Transactions] receivingPhone DEPOSITED $77.70 USD of Kin
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, sendingPhone).requireSuccess(t)
	sendingPhone.publiclyWithdraw777KinToCodeUserBetweenPrimaryAccounts(t, receivingPhone).requireSuccess(t)

	// [Cash Transactions] sendingPhone   SENT     $2.10 CAD of Kin
	// [Cash Transactions] receivingPhone RECEIVED $2.10 CAD of Kin
	happyPathGiftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, happyPathGiftCardAccount).requireSuccess(t)
	receivingPhone.receive42KinFromGiftCard(t, happyPathGiftCardAccount, false).requireSuccess(t)
	receivingPhone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)

	// [Cash Transactions] sendingPhone SENT $2.10 CAD of Kin
	expiredGiftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, expiredGiftCardAccount).requireSuccess(t)
	server.simulateExpiredGiftCard(t, expiredGiftCardAccount)

	// [Cash Transactions] sendingPhone SENT $2.10 CAD of Kin
	unclaimedGiftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, unclaimedGiftCardAccount).requireSuccess(t)

	// [Cash Transactions] receivingPhone SENT     $2.10 CAD of Kin
	// [Cash Transactions] receivingPhone RECEIVED $2.10 CAD of Kin
	selfClaimedGiftCardAccount := testutil.NewRandomAccount(t)
	receivingPhone.send42KinToGiftCardAccount(t, selfClaimedGiftCardAccount).requireSuccess(t)
	receivingPhone.receive42KinFromGiftCard(t, selfClaimedGiftCardAccount, false).requireSuccess(t)
	receivingPhone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)

	voidedGiftCardAccount := testutil.NewRandomAccount(t)
	sendingPhone.send42KinToGiftCardAccount(t, voidedGiftCardAccount).requireSuccess(t)
	sendingPhone.receive42KinFromGiftCard(t, voidedGiftCardAccount, true).requireSuccess(t)
	sendingPhone.receive42KinPrivatelyIntoOrganizer(t).requireSuccess(t)

	assert.Equal(t, transactionpb.AirdropResponse_OK, receivingPhone.requestAirdrop(t, transactionpb.AirdropType_GET_FIRST_KIN).Result)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulatePaymentRequest = true

	// [Verified Merchant] sendingPhone   SPENT $77.7 USD of Kin
	// [Verified Merchant] receivingPhone RECEIVED  $77.69 USD of Kin
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)

	sendingPhone.resetConfig()
	sendingPhone.conf.simulatePaymentRequest = true
	sendingPhone.conf.simulateAdditionalFees = true

	// [Verified Merchant] sendingPhone   SPENT $32.1 USD of Kin
	// [Verified Merchant] receivingPhone RECEIVED $29.69213 USD of Kin
	sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, merchantDomain).requireSuccess(t)

	// [Verified Merchant] sendingPhone   SPENT 123 Kin
	sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulatePaymentRequest = true
	receivingPhone.conf.simulateUnverifiedPaymentRequest = true

	// [Unverified Mechant] receivingPhone RECEIVED $77.69 USD of Kin
	receivingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)

	sendingPhone.resetConfig()
	receivingPhone.resetConfig()

	// [Cash Transactions] sendingPhone   WITHDREW  $32.1 USD of Kin
	// [Verified Merchant] receivingPhone RECEIVED $32.1 USD of Kin
	sendingPhone.publiclyWithdraw777KinToCodeUserBetweenRelationshipAccounts(t, merchantDomain, receivingPhone).requireSuccess(t)

	// [Cash Transactions] sendingPhone   WITHDREW  $32.1 USD of Kin
	// [Verified Merchant] receivingPhone RECEIVED $32.1 USD of Kin
	sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, merchantDomain).requireSuccess(t)

	// [Verified Merchant] receivingPhone RECEIVED 12,345 Kin
	server.simulateExternalDepositHistoryItem(t, receivingPhone.parentAccount, getTimelockVault(t, receivingPhone.getAuthorityForRelationshipAccount(t, merchantDomain)), kin.ToQuarks(12_345))

	sendingPhone.tip456KinToCodeUser(t, receivingPhone, twitterUsername).requireSuccess(t)

	chatMessageRecords, err := server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId(chat_util.CashTransactionsName, sendingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 10)

	protoChatMessage := getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.KIN, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 1.0, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 23.0, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(23), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[1])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_GAVE, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 4.20, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[2])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_WITHDREW, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[3])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_WITHDREW, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.KIN, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 1.0, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 123.0, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(123), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[4])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_WITHDREW, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[5])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[6])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[7])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[8])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_WITHDREW, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[9])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_WITHDREW, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 32.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(321), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId("example.com", sendingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 3)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SPENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[1])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SPENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 32.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(321), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[2])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SPENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.KIN, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 1.0, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 123.0, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(123), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId(chat_util.CashTransactionsName, receivingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 7)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 4.20, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[1])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[2])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.KIN, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 1.0, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 10_000_000_000.00, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(10_000_000_000), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[3])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[4])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[5])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SENT, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[6])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.CAD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.05, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 2.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(42), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId(chat_util.TipsName, sendingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 1)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_SENT_TIP, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 45.6, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(456), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId("example.com", receivingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 5)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.69, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.EqualValues(t, 77690000, protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[1])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 29.69213, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.EqualValues(t, 29692130, protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[2])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[3])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 32.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(321), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[4])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.KIN, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 1.0, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 12_345.0, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(12_345), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId("example.com", receivingPhone.parentAccount.PublicKey().ToBase58(), false))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 1)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.69, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.EqualValues(t, 77690000, protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessagesV1(server.ctx, chat_v1.GetChatId(chat_util.TipsName, receivingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 1)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_RECEIVED_TIP, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 45.6, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(456), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)
}
