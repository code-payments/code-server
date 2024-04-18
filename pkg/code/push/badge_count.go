package push

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/code-payments/code-server/pkg/code/common"
	code_data "github.com/code-payments/code-server/pkg/code/data"
	"github.com/code-payments/code-server/pkg/code/data/badgecount"
	"github.com/code-payments/code-server/pkg/code/data/login"
	push_data "github.com/code-payments/code-server/pkg/code/data/push"
	push_lib "github.com/code-payments/code-server/pkg/push"
)

// UpdateBadgeCount updates the badge count for an owner account to the latest value
//
// todo: Duplicated code with other send push utitilies
func UpdateBadgeCount(
	ctx context.Context,
	data code_data.Provider,
	pusher push_lib.Provider,
	owner *common.Account,
) error {
	log := logrus.StandardLogger().WithFields(logrus.Fields{
		"method": "SetBadgeCount",
		"owner":  owner.PublicKey().ToBase58(),
	})

	// todo: Propagate this logic to other push sending utilities once login
	//       detection is made public.
	loginRecord, err := data.GetLatestLoginByOwner(ctx, owner.PublicKey().ToBase58())
	if err == login.ErrLoginNotFound {
		return nil
	} else if err != nil {
		log.WithError(err).Warn("failure getting login record")
		return err
	}

	var badgeCount int
	badgeCountRecord, err := data.GetBadgeCount(ctx, owner.PublicKey().ToBase58())
	if err == nil {
		badgeCount = int(badgeCountRecord.BadgeCount)
	} else if err != badgecount.ErrBadgeCountNotFound {
		log.WithError(err).Warn("failure getting badge count record")
		return err
	}

	pushTokenRecords, err := getPushTokensForOwner(ctx, data, owner)
	if err != nil {
		log.WithError(err).Warn("failure getting push tokens for owner")
		return err
	}

	seenPushTokens := make(map[string]struct{})
	for _, pushTokenRecord := range pushTokenRecords {
		// Dedup push tokens, since they may appear more than once per app install
		if _, ok := seenPushTokens[pushTokenRecord.PushToken]; ok {
			continue
		}

		switch pushTokenRecord.TokenType {
		case push_data.TokenTypeFcmApns:
			log := log.WithField("push_token", pushTokenRecord.PushToken)

			// Legacy push tokens that don't map to an app install are skipped
			if pushTokenRecord.AppInstallId == nil {
				continue
			}

			// Only update the device with the latest app login for the owner
			if *pushTokenRecord.AppInstallId != loginRecord.AppInstallId {
				continue
			}

			log.Debugf("updating badge count on device to %d", badgeCount)

			// Try push to update badge count
			pushErr := pusher.SetAPNSBadgeCount(
				ctx,
				pushTokenRecord.PushToken,
				badgeCount,
			)

			if pushErr != nil {
				isValid, onPushErr := onPushError(ctx, data, pusher, pushTokenRecord)

				log.WithError(pushErr).
					WithFields(logrus.Fields{
						"on_push_error": onPushErr,
						"is_valid":      isValid,
					}).
					Warn("failure sending push notification")

				if onPushErr != nil {
					return fmt.Errorf("failed to handle push error (%w): %w", pushErr, onPushErr)
				} else if isValid {
					return fmt.Errorf("failed to push to valid token: %w", pushErr)
				}
			}
		}

		seenPushTokens[pushTokenRecord.PushToken] = struct{}{}
	}

	return nil
}
