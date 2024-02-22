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
	// Section: Chats
	//

	// Chat Titles

	ChatTitleCashTransactions = "title.chat.cashTransactions"
	ChatTitleCodeTeam         = "title.chat.codeTeam"
	ChatTitlePayments         = "title.chat.payments"

	// Message Bodies

	ChatMessageReferralBonus = "subtitle.chat.referralBonus"
	ChatMessageWelcomeBonus  = "subtitle.chat.welcomeBonus"

	ChatMessageUsdcDeposited      = "subtitle.chat.usdcDeposited"
	ChatMessageUsdcBeingConverted = "subtitle.chat.usdcBeingConverted"
	ChatMessageKinAvailableForUse = "subtitle.chat.kinAvailableForUse"

	//
	// Verbs
	//

	VerbGave      = "subtitle.youGave"
	VerbReceived  = "subtitle.youReceived"
	VerbWithdrew  = "subtitle.youWithdrew"
	VerbDeposited = "subtitle.youDeposited"
	VerbSent      = "subtitle.youSent"
	VerbSpent     = "subtitle.youSpent"
	VerbPaid      = "subtitle.youPaid"
	VerbPurchased = "subtitle.youPurchased"
	VerbReturned  = "subtitle.wasReturnedToYou"
)

var (
	bundleMu sync.RWMutex
	bundle   *i18n.Bundle
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

	newBundle := i18n.NewBundle(language.English)

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

// LocalizeKey localizes a key to the corresponding string in the provided locale.
// An optional set of string parameters can be provided to be replaced in the string.
// Currenctly, these arguments must be localized outside of this function.
//
// todo: Generic argument handling, so all localization can happen in here
func LocalizeKey(locale language.Tag, key string, args ...string) (string, error) {
	bundleMu.RLock()
	defer bundleMu.RUnlock()

	if bundle == nil {
		return "", errors.New("localization bundle not configured")
	}

	localizer := i18n.NewLocalizer(bundle, locale.String())

	localizeConfigInvite := i18n.LocalizeConfig{
		MessageID: key,
	}

	value, err := localizer.Localize(&localizeConfigInvite)
	if err != nil {
		return "", err
	}

	for _, arg := range args {
		value = strings.Replace(value, "%@", arg, 1)
	}
	return value, nil
}
