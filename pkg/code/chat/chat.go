package chat

import "github.com/code-payments/code-server/pkg/code/localization"

const (
	CashTransactionsName = "Cash Transactions" // Renamed to Cash Payments on client
	CodeTeamName         = "Code Team"
	KinPurchasesName     = "Kin Purchases"
	PaymentsName         = "Payments" // Renamed to Web Payments on client
	TipsName             = "Tips"
	TwoWayChatName       = "Two Way Chat"

	// Test chats used for unit/integration testing only
	TestCantMuteName        = "TestCantMute"
	TestCantUnsubscribeName = "TestCantUnsubscribe"
)

var (
	InternalChatProperties = map[string]struct {
		TitleLocalizationKey string
		CanMute              bool
		CanUnsubscribe       bool
	}{
		CashTransactionsName: {
			TitleLocalizationKey: localization.ChatTitleCashTransactions,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		CodeTeamName: {
			TitleLocalizationKey: localization.ChatTitleCodeTeam,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		KinPurchasesName: {
			TitleLocalizationKey: localization.ChatTitleKinPurchases,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		PaymentsName: {
			TitleLocalizationKey: localization.ChatTitlePayments,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		TipsName: {
			TitleLocalizationKey: localization.ChatTitleTips,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		TwoWayChatName: {
			TitleLocalizationKey: localization.ChatTitleTwoWay,
			CanMute:              true,
			CanUnsubscribe:       false,
		},

		TestCantMuteName: {
			TitleLocalizationKey: "n/a",
			CanMute:              false,
			CanUnsubscribe:       true,
		},
		TestCantUnsubscribeName: {
			TitleLocalizationKey: "n/a",
			CanMute:              true,
			CanUnsubscribe:       false,
		},
	}
)
