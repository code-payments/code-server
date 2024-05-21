package localization

import (
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

const (
	//
	// Section: Legacy Push Notifications (don't follow key conventions)
	//

	PushTitleDepositReceived    = "push.title.depositReceived"
	PushSubtitleDepositReceived = "push.subtitle.depositReceived"

	PushTitleKinReturned    = "push.title.kinReturned"
	PushSubtitleKinReturned = "push.subtitle.kinReturned"

	//
	// Section: Core
	//

	CoreOfKin = "core.ofKin"
	CoreKin   = "core.kin"

	//
	// Section: Pushes
	//

	PushTitleTwitterAccountConnected    = "title.push.twitterAccountConnected"
	PushSubtitleTwitterAccountConnected = "subtitle.push.twitterAccountConnected"

	//
	// Section: Chats
	//

	// Chat Titles

	ChatTitleCashTransactions = "title.chat.cashTransactions"
	ChatTitleCodeTeam         = "title.chat.codeTeam"
	ChatTitleKinPurchases     = "title.chat.kinPurchases"
	ChatTitlePayments         = "title.chat.payments"
	ChatTitleTips             = "title.chat.tips"

	// Message Bodies

	ChatMessageReferralBonus = "subtitle.chat.referralBonus"
	ChatMessageWelcomeBonus  = "subtitle.chat.welcomeBonus"

	ChatMessageUsdcDeposited      = "subtitle.chat.usdcDeposited"
	ChatMessageUsdcBeingConverted = "subtitle.chat.usdcBeingConverted"
	ChatMessageKinAvailableForUse = "subtitle.chat.kinAvailableForUse"

	//
	// Verbs
	//

	VerbGave        = "subtitle.youGave"
	VerbReceived    = "subtitle.youReceived"
	VerbWithdrew    = "subtitle.youWithdrew"
	VerbDeposited   = "subtitle.youDeposited"
	VerbSent        = "subtitle.youSent"
	VerbSpent       = "subtitle.youSpent"
	VerbPaid        = "subtitle.youPaid"
	VerbPurchased   = "subtitle.youPurchased"
	VerbReturned    = "subtitle.wasReturnedToYou"
	VerbReceivedTip = "subtitle.someoneTippedYou"
	VerbSentTip     = "subtitle.youTipped"
)

var (
	bundleMu sync.RWMutex
	bundle   *i18n.Bundle

	defaultLocale = language.English
)

// LoadKeys loads localization key-value pairs from the provided directory.
//
// todo: we'll want to improve this, but just getting something quick up to setup his localization package.
func LoadKeys(directory string) error {
	if !strings.HasSuffix(directory, "/") {
		directory = directory + "/"
	}

	bundleMu.Lock()
	defer bundleMu.Unlock()

	newBundle := i18n.NewBundle(defaultLocale)

	dirEntries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}

	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() && strings.HasSuffix(dirEntry.Name(), ".json") {
			_, err = newBundle.LoadMessageFile(directory + dirEntry.Name())
			if err != nil {
				return err
			}
		}
	}

	bundle = newBundle
	return nil
}

// LoadTestKeys is a utility for injecting test localization keys
func LoadTestKeys(kvsByLocale map[language.Tag]map[string]string) {
	bundleMu.Lock()
	defer bundleMu.Unlock()

	newBundle := i18n.NewBundle(language.English)

	for locale, kvs := range kvsByLocale {
		messages := make([]*i18n.Message, 0)
		for k, v := range kvs {
			messages = append(messages, &i18n.Message{
				ID:    k,
				Other: v,
			})
		}
		newBundle.AddMessages(locale, messages...)
	}

	bundle = newBundle
}

// ResetKeys resets localization to an empty mapping
func ResetKeys() {
	bundleMu.Lock()
	defer bundleMu.Unlock()

	bundle = i18n.NewBundle(language.English)
}

func localizeKey(locale language.Tag, key string, args ...string) (string, *language.Tag, error) {
	bundleMu.RLock()
	defer bundleMu.RUnlock()

	if bundle == nil {
		return "", nil, errors.New("localization bundle not configured")
	}

	localizeConfigInvite := i18n.LocalizeConfig{
		MessageID: key,
	}

	localizer := i18n.NewLocalizer(bundle, locale.String())
	localized, tag, err := localizer.LocalizeWithTag(&localizeConfigInvite)
	switch err.(type) {
	case *i18n.MessageNotFoundErr:
		// Fall back to default locale if the key doesn't exist for the requested
		// locale
		localizer := i18n.NewLocalizer(bundle, defaultLocale.String())
		localized, tag, err = localizer.LocalizeWithTag(&localizeConfigInvite)
		if err != nil {
			return "", nil, err
		}
	case nil:
	default:
		return "", nil, err
	}

	for _, arg := range args {
		localized = strings.Replace(localized, "%@", arg, 1)
	}
	return localized, &tag, nil
}

// Localize localizes a key to the corresponding string in the provided locale.
// An optional set of string parameters can be provided to be replaced in the string.
// Currenctly, these arguments must be localized outside of this function.
//
// todo: Generic argument handling, so all localization can happen in here
func Localize(locale language.Tag, key string, args ...string) (string, error) {
	localized, _, err := localizeKey(locale, key, args...)
	return localized, err
}

// LocalizeWithFallback is like Localize, but returns defaultValue on error.
func LocalizeWithFallback(locale language.Tag, defaultValue, key string, args ...string) string {
	localized, _, err := localizeKey(locale, key, args...)
	if err != nil {
		return defaultValue
	}
	return localized
}
