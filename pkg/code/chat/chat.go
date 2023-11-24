package chat

import "github.com/code-payments/code-server/pkg/code/localization"

var (
	InternalChatProperties = map[string]struct {
		TitleLocalizationKey string
		CanMute              bool
		CanUnsubscribe       bool
	}{
		CashTransactionsName: {
			TitleLocalizationKey: localization.ChatTitleCashTransactions,
			CanMute:              false,
			CanUnsubscribe:       false,
		},
		CodeTeamName: {
			TitleLocalizationKey: localization.ChatTitleCodeTeam,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		PaymentsName: {
			TitleLocalizationKey: localization.ChatTitlePayments,
			CanMute:              false,
			CanUnsubscribe:       false,
		},
	}
)
