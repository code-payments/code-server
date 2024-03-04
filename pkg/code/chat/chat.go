package chat

import "github.com/code-payments/code-server/pkg/code/localization"

// Defining constants for localisation keys to ensure consistency and reduce typos.
const (
	LocalKeyCashTransactions = localization.ChatTitleCashTransactions
	LocalKeyCodeTeam         = localization.ChatTitleCodeTeam
	LocalKeyPayments         = localization.ChatTitlePayments
)

// ChatProperty holds the properties of a chat including title localisation, mutability, and unsubscribe capability.
type ChatProperty struct {
	TitleLocalizationKey string `json:"title"` // For client-side naming, use struct tags if needed.
	CanMute              bool   `json:"canMute"`
	CanUnsubscribe       bool   `json:"canUnsubscribe"`
}

var (
	// InternalChatProperties defines properties for internal chats.
	InternalChatProperties = map[string]ChatProperty{
		"Cash Transactions": { // Renamed to Cash Payments on client
			TitleLocalizationKey: LocalKeyCashTransactions,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		"Code Team": {
			TitleLocalizationKey: LocalKeyCodeTeam,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
		"Payments": { // Renamed to Web Payments on client
			TitleLocalizationKey: LocalKeyPayments,
			CanMute:              true,
			CanUnsubscribe:       false,
		},
	}

	// TestChatProperties defines properties for test chats, separating them from production configurations.
	TestChatProperties = map[string]ChatProperty{
		"TestCantMute": {
			TitleLocalizationKey: "n/a",
			CanMute:              false,
			CanUnsubscribe:       true,
		},
		"TestCantUnsubscribe": {
			TitleLocalizationKey: "n/a",
			CanMute:              true,
			CanUnsubscribe:       false,
		},
	}
)
