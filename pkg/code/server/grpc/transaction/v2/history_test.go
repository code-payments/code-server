package transaction_v2

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	chatpb "github.com/code-payments/code-protobuf-api/generated/go/chat/v1"
	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	transactionpb "github.com/code-payments/code-protobuf-api/generated/go/transaction/v2"

	chat_util "github.com/code-payments/code-server/pkg/code/chat"
	"github.com/code-payments/code-server/pkg/code/data/chat"
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

	server.generateAvailableNonces(t, 1000)
	server.setupAirdropper(t, kin.ToQuarks(1_500_000_000))

	amountForPrivacyMigration := kin.ToQuarks(23)
	legacyTimelockVault, err := sendingPhone.parentAccount.ToTimelockVault(timelock_token_v1.DataVersionLegacy)
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
	// [Verified Merchant] receivingPhone PAID  $77.7 USD of Kin
	sendingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)
	receivingPhone.deposit777KinIntoOrganizer(t).requireSuccess(t)

	// [Verified Merchant] sendingPhone   SPENT $32.1 USD of Kin
	// [Verified Merchant] receivingPhone PAID  $32.1 USD of Kin
	sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, merchantDomain).requireSuccess(t)

	// [Verified Merchant] sendingPhone   SPENT 123 Kin
	sendingPhone.privatelyWithdraw123KinToExternalWallet(t).requireSuccess(t)

	receivingPhone.resetConfig()
	receivingPhone.conf.simulatePaymentRequest = true
	receivingPhone.conf.simulateUnverifiedPaymentRequest = true

	// [Unverified Mechant] receivingPhone PAID $77.7 USD of Kin
	receivingPhone.privatelyWithdraw777KinToCodeUser(t, receivingPhone).requireSuccess(t)

	sendingPhone.resetConfig()
	receivingPhone.resetConfig()

	sendingPhone.publiclyWithdraw777KinToCodeUserBetweenRelationshipAccounts(t, merchantDomain, receivingPhone).requireSuccess(t)
	sendingPhone.privatelyWithdraw321KinToCodeUserRelationshipAccount(t, receivingPhone, merchantDomain).requireSuccess(t)

	//
	// New chat assertions below
	//

	chatMessageRecords, err := server.data.GetAllChatMessages(server.ctx, chat.GetChatId(chat_util.CashTransactionsName, sendingPhone.parentAccount.PublicKey().ToBase58(), true))
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

	chatMessageRecords, err = server.data.GetAllChatMessages(server.ctx, chat.GetChatId("example.com", sendingPhone.parentAccount.PublicKey().ToBase58(), true))
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

	chatMessageRecords, err = server.data.GetAllChatMessages(server.ctx, chat.GetChatId(chat_util.CashTransactionsName, receivingPhone.parentAccount.PublicKey().ToBase58(), true))
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

	chatMessageRecords, err = server.data.GetAllChatMessages(server.ctx, chat.GetChatId("example.com", receivingPhone.parentAccount.PublicKey().ToBase58(), true))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 4)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_PAID, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[1])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_PAID, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 32.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(321), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[2])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[3])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_DEPOSITED, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 32.1, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(321), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	chatMessageRecords, err = server.data.GetAllChatMessages(server.ctx, chat.GetChatId("example.com", receivingPhone.parentAccount.PublicKey().ToBase58(), false))
	require.NoError(t, err)
	require.Len(t, chatMessageRecords, 1)

	protoChatMessage = getProtoChatMessage(t, chatMessageRecords[0])
	require.Len(t, protoChatMessage.Content, 1)
	require.NotNil(t, protoChatMessage.Content[0].GetExchangeData())
	assert.Equal(t, chatpb.ExchangeDataContent_PAID, protoChatMessage.Content[0].GetExchangeData().Verb)
	assert.EqualValues(t, currency_lib.USD, protoChatMessage.Content[0].GetExchangeData().GetExact().Currency)
	assert.Equal(t, 0.1, protoChatMessage.Content[0].GetExchangeData().GetExact().ExchangeRate)
	assert.Equal(t, 77.7, protoChatMessage.Content[0].GetExchangeData().GetExact().NativeAmount)
	assert.Equal(t, kin.ToQuarks(777), protoChatMessage.Content[0].GetExchangeData().GetExact().Quarks)

	//
	// Legacy GetPaymentHistory RPC assertions below
	//

	items := sendingPhone.getPaymentHistory(t)
	require.Len(t, items, 14)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[0].PaymentType)
	assert.Equal(t, amountForPrivacyMigration, items[0].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.KIN, items[0].ExchangeData.Currency)
	assert.Equal(t, 1.0, items[0].ExchangeData.ExchangeRate)
	assert.EqualValues(t, kin.FromQuarks(amountForPrivacyMigration), items[0].ExchangeData.NativeAmount)
	assert.False(t, items[0].IsWithdraw)
	assert.True(t, items[0].IsDeposit)
	assert.False(t, items[0].IsRemoteSend)
	assert.False(t, items[0].IsReturned)
	assert.False(t, items[0].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[0].AirdropType)
	assert.False(t, items[0].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[1].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[1].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[1].ExchangeData.Currency)
	assert.Equal(t, 0.1, items[1].ExchangeData.ExchangeRate)
	assert.Equal(t, 4.2, items[1].ExchangeData.NativeAmount)
	assert.False(t, items[1].IsWithdraw)
	assert.False(t, items[1].IsDeposit)
	assert.False(t, items[1].IsRemoteSend)
	assert.False(t, items[1].IsReturned)
	assert.False(t, items[1].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[1].AirdropType)
	assert.False(t, items[1].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[2].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[2].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[2].ExchangeData.Currency)
	assert.Equal(t, 0.1, items[2].ExchangeData.ExchangeRate)
	assert.Equal(t, 77.7, items[2].ExchangeData.NativeAmount)
	assert.True(t, items[2].IsWithdraw)
	assert.False(t, items[2].IsDeposit)
	assert.False(t, items[2].IsRemoteSend)
	assert.False(t, items[2].IsReturned)
	assert.False(t, items[2].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[2].AirdropType)
	assert.False(t, items[2].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[3].PaymentType)
	assert.Equal(t, kin.ToQuarks(123), items[3].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.KIN, items[3].ExchangeData.Currency)
	assert.EqualValues(t, 1, items[3].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 123, items[3].ExchangeData.NativeAmount)
	assert.True(t, items[3].IsWithdraw)
	assert.False(t, items[3].IsDeposit)
	assert.False(t, items[3].IsRemoteSend)
	assert.False(t, items[3].IsReturned)
	assert.False(t, items[3].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[3].AirdropType)
	assert.False(t, items[3].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[4].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[4].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[4].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[4].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[4].ExchangeData.NativeAmount)
	assert.True(t, items[4].IsWithdraw)
	assert.False(t, items[4].IsDeposit)
	assert.False(t, items[4].IsRemoteSend)
	assert.False(t, items[4].IsReturned)
	assert.False(t, items[4].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[4].AirdropType)
	assert.False(t, items[4].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[5].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[5].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[5].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[5].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[5].ExchangeData.NativeAmount)
	assert.False(t, items[5].IsWithdraw)
	assert.False(t, items[5].IsDeposit)
	assert.True(t, items[5].IsRemoteSend)
	assert.False(t, items[5].IsReturned)
	assert.False(t, items[5].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[5].AirdropType)
	assert.False(t, items[5].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[6].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[6].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[6].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[6].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[6].ExchangeData.NativeAmount)
	assert.False(t, items[6].IsWithdraw)
	assert.False(t, items[6].IsDeposit)
	assert.True(t, items[6].IsRemoteSend)
	assert.False(t, items[6].IsReturned)
	assert.False(t, items[6].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[6].AirdropType)
	assert.False(t, items[6].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[7].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[7].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[7].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[7].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[7].ExchangeData.NativeAmount)
	assert.False(t, items[7].IsWithdraw)
	assert.False(t, items[7].IsDeposit)
	assert.True(t, items[7].IsRemoteSend)
	assert.True(t, items[7].IsReturned)
	assert.False(t, items[7].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[7].AirdropType)
	assert.False(t, items[7].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[8].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[8].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[8].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[8].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[8].ExchangeData.NativeAmount)
	assert.False(t, items[8].IsWithdraw)
	assert.False(t, items[8].IsDeposit)
	assert.True(t, items[8].IsRemoteSend)
	assert.False(t, items[8].IsReturned)
	assert.False(t, items[8].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[8].AirdropType)
	assert.False(t, items[8].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[9].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[9].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[9].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[9].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[9].ExchangeData.NativeAmount)
	assert.True(t, items[9].IsWithdraw)
	assert.False(t, items[9].IsDeposit)
	assert.False(t, items[9].IsRemoteSend)
	assert.False(t, items[9].IsReturned)
	assert.False(t, items[9].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[9].AirdropType)
	assert.True(t, items[9].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[10].PaymentType)
	assert.Equal(t, kin.ToQuarks(321), items[10].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[10].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[10].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 32.1, items[10].ExchangeData.NativeAmount)
	assert.True(t, items[10].IsWithdraw)
	assert.False(t, items[10].IsDeposit)
	assert.False(t, items[10].IsRemoteSend)
	assert.False(t, items[10].IsReturned)
	assert.False(t, items[10].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[10].AirdropType)
	assert.True(t, items[10].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[11].PaymentType)
	assert.Equal(t, kin.ToQuarks(123), items[11].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.KIN, items[11].ExchangeData.Currency)
	assert.EqualValues(t, 1, items[11].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 123, items[11].ExchangeData.NativeAmount)
	assert.True(t, items[11].IsWithdraw)
	assert.False(t, items[11].IsDeposit)
	assert.False(t, items[11].IsRemoteSend)
	assert.False(t, items[11].IsReturned)
	assert.False(t, items[11].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[11].AirdropType)
	assert.True(t, items[11].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[12].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[12].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[12].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[12].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[12].ExchangeData.NativeAmount)
	assert.True(t, items[12].IsWithdraw)
	assert.False(t, items[12].IsDeposit)
	assert.False(t, items[12].IsRemoteSend)
	assert.False(t, items[12].IsReturned)
	assert.False(t, items[12].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[12].AirdropType)
	assert.False(t, items[12].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[13].PaymentType)
	assert.Equal(t, kin.ToQuarks(321), items[13].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[13].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[13].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 32.1, items[13].ExchangeData.NativeAmount)
	assert.True(t, items[13].IsWithdraw)
	assert.False(t, items[13].IsDeposit)
	assert.False(t, items[13].IsRemoteSend)
	assert.False(t, items[13].IsReturned)
	assert.False(t, items[13].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[13].AirdropType)
	assert.False(t, items[13].IsMicroPayment)

	items = receivingPhone.getPaymentHistory(t)
	require.Len(t, items, 13)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[0].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[0].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[0].ExchangeData.Currency)
	assert.Equal(t, 0.1, items[0].ExchangeData.ExchangeRate)
	assert.Equal(t, 4.2, items[0].ExchangeData.NativeAmount)
	assert.False(t, items[0].IsWithdraw)
	assert.False(t, items[0].IsDeposit)
	assert.False(t, items[0].IsRemoteSend)
	assert.False(t, items[0].IsReturned)
	assert.False(t, items[0].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[0].AirdropType)
	assert.False(t, items[0].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[1].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[1].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[1].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[1].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[1].ExchangeData.NativeAmount)
	assert.False(t, items[1].IsWithdraw)
	assert.True(t, items[1].IsDeposit)
	assert.False(t, items[1].IsRemoteSend)
	assert.False(t, items[1].IsReturned)
	assert.False(t, items[1].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[1].AirdropType)
	assert.False(t, items[1].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[2].PaymentType)
	assert.Equal(t, kin.ToQuarks(10_000_000_000), items[2].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.KIN, items[2].ExchangeData.Currency)
	assert.Equal(t, 1.0, items[2].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 10_000_000_000, items[2].ExchangeData.NativeAmount)
	assert.False(t, items[2].IsWithdraw)
	assert.True(t, items[2].IsDeposit)
	assert.False(t, items[2].IsRemoteSend)
	assert.False(t, items[2].IsReturned)
	assert.False(t, items[2].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[1].AirdropType)
	assert.False(t, items[2].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[3].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[3].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[3].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[3].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[3].ExchangeData.NativeAmount)
	assert.False(t, items[3].IsWithdraw)
	assert.True(t, items[3].IsDeposit)
	assert.False(t, items[3].IsRemoteSend)
	assert.False(t, items[3].IsReturned)
	assert.False(t, items[3].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[3].AirdropType)
	assert.False(t, items[3].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[4].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[4].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[4].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[4].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[4].ExchangeData.NativeAmount)
	assert.False(t, items[4].IsWithdraw)
	assert.False(t, items[4].IsDeposit)
	assert.True(t, items[4].IsRemoteSend)
	assert.False(t, items[4].IsReturned)
	assert.False(t, items[4].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[4].AirdropType)
	assert.False(t, items[4].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_SEND, items[5].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[5].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[5].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[5].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[5].ExchangeData.NativeAmount)
	assert.False(t, items[5].IsWithdraw)
	assert.False(t, items[5].IsDeposit)
	assert.True(t, items[5].IsRemoteSend)
	assert.False(t, items[5].IsReturned)
	assert.False(t, items[5].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[5].AirdropType)
	assert.False(t, items[5].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[6].PaymentType)
	assert.Equal(t, kin.ToQuarks(42), items[6].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.CAD, items[6].ExchangeData.Currency)
	assert.EqualValues(t, 0.05, items[6].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 2.1, items[6].ExchangeData.NativeAmount)
	assert.False(t, items[6].IsWithdraw)
	assert.False(t, items[6].IsDeposit)
	assert.True(t, items[6].IsRemoteSend)
	assert.False(t, items[6].IsReturned)
	assert.False(t, items[6].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[6].AirdropType)
	assert.False(t, items[6].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[7].PaymentType)
	assert.Equal(t, kin.ToQuarks(10)+kin.ToQuarks(1), items[7].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[7].ExchangeData.Currency)
	assert.Equal(t, 0.1, items[7].ExchangeData.ExchangeRate)
	assert.Equal(t, 1.0, items[7].ExchangeData.NativeAmount)
	assert.False(t, items[7].IsWithdraw)
	assert.True(t, items[7].IsDeposit)
	assert.False(t, items[7].IsRemoteSend)
	assert.False(t, items[7].IsReturned)
	assert.True(t, items[7].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_GET_FIRST_KIN, items[7].AirdropType)
	assert.False(t, items[7].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[8].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[8].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[8].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[8].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[8].ExchangeData.NativeAmount)
	assert.False(t, items[8].IsWithdraw)
	assert.True(t, items[8].IsDeposit)
	assert.False(t, items[8].IsRemoteSend)
	assert.False(t, items[8].IsReturned)
	assert.False(t, items[8].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[8].AirdropType)
	assert.True(t, items[8].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[9].PaymentType)
	assert.Equal(t, kin.ToQuarks(321), items[9].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[9].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[9].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 32.1, items[9].ExchangeData.NativeAmount)
	assert.False(t, items[9].IsWithdraw)
	assert.True(t, items[9].IsDeposit)
	assert.False(t, items[9].IsRemoteSend)
	assert.False(t, items[9].IsReturned)
	assert.False(t, items[9].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[9].AirdropType)
	assert.True(t, items[9].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[10].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[10].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[10].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[10].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[10].ExchangeData.NativeAmount)
	assert.False(t, items[10].IsWithdraw)
	assert.True(t, items[10].IsDeposit)
	assert.False(t, items[10].IsRemoteSend)
	assert.False(t, items[10].IsReturned)
	assert.False(t, items[10].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[10].AirdropType)
	assert.True(t, items[10].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[11].PaymentType)
	assert.Equal(t, kin.ToQuarks(777), items[11].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[11].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[11].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 77.7, items[11].ExchangeData.NativeAmount)
	assert.False(t, items[11].IsWithdraw)
	assert.True(t, items[11].IsDeposit)
	assert.False(t, items[11].IsRemoteSend)
	assert.False(t, items[11].IsReturned)
	assert.False(t, items[11].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[11].AirdropType)
	assert.False(t, items[11].IsMicroPayment)

	assert.Equal(t, transactionpb.PaymentHistoryItem_RECEIVE, items[12].PaymentType)
	assert.Equal(t, kin.ToQuarks(321), items[12].ExchangeData.Quarks)
	assert.EqualValues(t, currency_lib.USD, items[12].ExchangeData.Currency)
	assert.EqualValues(t, 0.1, items[12].ExchangeData.ExchangeRate)
	assert.EqualValues(t, 32.1, items[12].ExchangeData.NativeAmount)
	assert.False(t, items[12].IsWithdraw)
	assert.True(t, items[12].IsDeposit)
	assert.False(t, items[12].IsRemoteSend)
	assert.False(t, items[12].IsReturned)
	assert.False(t, items[12].IsAirdrop)
	assert.Equal(t, transactionpb.AirdropType_UNKNOWN, items[12].AirdropType)
	assert.False(t, items[12].IsMicroPayment)
}

func TestGetPaymentHistory_NoPayments(t *testing.T) {
	_, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	assert.Empty(t, phone.getPaymentHistory(t))
}

func TestGetPaymentHistory_UnauthenticatedAccess(t *testing.T) {
	_, phone, _, cleanup := setupTestEnv(t, &testOverrides{})
	defer cleanup()

	maliciousAccount := testutil.NewRandomAccount(t)

	req := &transactionpb.GetPaymentHistoryRequest{
		Owner: phone.parentAccount.ToProto(),
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)
	req.Signature = &commonpb.Signature{
		Value: ed25519.Sign(maliciousAccount.PrivateKey().ToBytes(), reqBytes),
	}

	_, err = phone.client.GetPaymentHistory(phone.ctx, req)
	testutil.AssertStatusErrorWithCode(t, err, codes.Unauthenticated)
}
