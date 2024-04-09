package async_user

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	push_util "github.com/code-payments/code-server/pkg/code/push"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	twitter_lib "github.com/code-payments/code-server/pkg/twitter"
)

const (
	tipCardRegistrationPrefix = "CodeAccount"
	maxTweetSearchResults     = 100 // maximum allowed
)

var (
	errTwitterInvalidRegistrationValue = errors.New("twitter registration value is invalid")
	errTwitterRegistrationNotFound     = errors.New("twitter registration not found")
)

func (p *service) twitterRegistrationWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "twitterRegistrationWorker")

	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__user_service__handle_twitter_registration")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			err = p.processNewTwitterRegistrations(tracedCtx)
			if err != nil {
				m.NoticeError(err)
				log.WithError(err).Warn("failure processing new twitter registrations")
			}
			return err
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) twitterUserInfoUpdateWorker(serviceCtx context.Context, interval time.Duration) error {
	log := p.log.WithField("method", "twitterUserInfoUpdateWorker")

	delay := interval

	err := retry.Loop(
		func() (err error) {
			time.Sleep(delay)

			nr := serviceCtx.Value(metrics.NewRelicContextKey).(*newrelic.Application)
			m := nr.StartTransaction("async__user_service__handle_twitter_user_info_update")
			defer m.End()
			tracedCtx := newrelic.NewContext(serviceCtx, m)

			// todo: configurable parameters
			records, err := p.data.GetStaleTwitterUsers(tracedCtx, 7*24*time.Hour, 32)
			if err == twitter.ErrUserNotFound {
				return nil
			} else if err != nil {
				m.NoticeError(err)
				log.WithError(err).Warn("failure getting stale twitter users")
				return err
			}

			for _, record := range records {
				err := p.refreshTwitterUserInfo(tracedCtx, record.Username)
				if err != nil {
					m.NoticeError(err)
					log.WithError(err).Warn("failure refreshing twitter user info")
					return err
				}
			}

			return nil
		},
		retry.NonRetriableErrors(context.Canceled),
	)

	return err
}

func (p *service) processNewTwitterRegistrations(ctx context.Context) error {
	tweets, err := p.findNewRegistrationTweets(ctx)
	if err != nil {
		return errors.Wrap(err, "error finding new registration tweets")
	}

	for _, tweet := range tweets {
		if tweet.AdditionalMetadata.Author == nil {
			return errors.Errorf("author missing in tweet %s", tweet.ID)
		}

		// Attempt to find a verified tip account from the registration tweet
		tipAccount, registrationNonce, err := p.findVerifiedTipAccountRegisteredInTweet(ctx, tweet)
		switch err {
		case nil:
		case errTwitterInvalidRegistrationValue, errTwitterRegistrationNotFound:
			continue
		default:
			return errors.Wrapf(err, "unexpected error processing tweet %s", tweet.ID)
		}

		// Save the updated tipping information
		err = p.data.ExecuteInTx(ctx, sql.LevelDefault, func(ctx context.Context) error {
			err = p.data.MarkTwitterNonceAsUsed(ctx, tweet.ID, *registrationNonce)
			if err != nil {
				return err
			}

			err = p.updateCachedTwitterUser(ctx, tweet.AdditionalMetadata.Author, tipAccount)
			if err != nil {
				return err
			}

			err = p.data.MarkTweetAsProcessed(ctx, tweet.ID)
			if err != nil {
				return err
			}

			return nil
		})

		switch err {
		case nil:
			go push_util.SendTwitterAccountConnectedPushNotification(ctx, p.data, p.pusher, tipAccount)
		case twitter.ErrDuplicateTipAddress, twitter.ErrDuplicateNonce:
			// Any race conditions with duplicate nonces or tip addresses will are ignored
			//
			// todo: In the future, support multiple tip address mappings
		default:
			return errors.Wrap(err, "error saving new registration")
		}
	}

	return nil
}

func (p *service) refreshTwitterUserInfo(ctx context.Context, username string) error {
	user, err := p.twitterClient.GetUserByUsername(ctx, username)
	if err != nil {
		return errors.Wrap(err, "error getting user info from twitter")
	}

	err = p.updateCachedTwitterUser(ctx, user, nil)
	if err != nil {
		return errors.Wrap(err, "error updating cached user state")
	}
	return nil
}

func (p *service) updateCachedTwitterUser(ctx context.Context, user *twitter_lib.User, newTipAccount *common.Account) error {
	mu := p.userLocks.Get([]byte(user.Username))
	mu.Lock()
	defer mu.Unlock()

	record, err := p.data.GetTwitterUserByUsername(ctx, user.Username)
	switch err {
	case twitter.ErrUserNotFound:
		if newTipAccount == nil {
			return errors.New("tip account must be present for newly registered twitter users")
		}

		record = &twitter.Record{
			Username: user.Username,
		}

		fallthrough
	case nil:
		record.Name = user.Name
		record.ProfilePicUrl = user.ProfileImageUrl
		record.VerifiedType = toProtoVerifiedType(user.VerifiedType)
		record.FollowerCount = uint32(user.PublicMetrics.FollowersCount)

		if newTipAccount != nil {
			record.TipAddress = newTipAccount.PublicKey().ToBase58()
		}
	default:
		return errors.Wrap(err, "error getting cached twitter user")
	}

	err = p.data.SaveTwitterUser(ctx, record)
	switch err {
	case nil, twitter.ErrDuplicateTipAddress:
		return err
	default:
		return errors.Wrap(err, "error updating cached twitter user")
	}
}

func (p *service) findNewRegistrationTweets(ctx context.Context) ([]*twitter_lib.Tweet, error) {
	var pageToken *string
	var res []*twitter_lib.Tweet
	for {
		tweets, nextPageToken, err := p.twitterClient.SearchRecentTweets(
			ctx,
			tipCardRegistrationPrefix,
			maxTweetSearchResults,
			pageToken,
		)
		if err != nil {
			return nil, errors.Wrap(err, "error searching tweets")
		}

		for _, tweet := range tweets {
			isTweetProcessed, err := p.data.IsTweetProcessed(ctx, tweet.ID)
			if err != nil {
				return nil, errors.Wrap(err, "error checking if tweet is processed")
			} else if isTweetProcessed {
				// Found a checkpoint
				return res, nil
			}

			res = append([]*twitter_lib.Tweet{tweet}, res...)
		}

		if nextPageToken == nil {
			return res, nil
		}
		pageToken = nextPageToken
	}
}

func (p *service) findVerifiedTipAccountRegisteredInTweet(ctx context.Context, tweet *twitter_lib.Tweet) (*common.Account, *uuid.UUID, error) {
	tweetParts := strings.Fields(tweet.Text)
	for _, tweetPart := range tweetParts {
		// Look for the well-known prefix to indicate a potential registration value

		if !strings.HasPrefix(tweetPart, tipCardRegistrationPrefix) {
			continue
		}

		// Parse out the individual components of the registration value

		tweetPart = strings.TrimSuffix(tweetPart, ".")
		registrationParts := strings.Split(tweetPart, ":")
		if len(registrationParts) != 4 {
			return nil, nil, errTwitterInvalidRegistrationValue
		}

		addressString := registrationParts[1]
		nonceString := registrationParts[2]
		signatureString := registrationParts[3]

		decodedAddress, err := base58.Decode(addressString)
		if err != nil {
			return nil, nil, errTwitterInvalidRegistrationValue
		}
		if len(decodedAddress) != 32 {
			return nil, nil, errTwitterInvalidRegistrationValue
		}
		tipAccount, _ := common.NewAccountFromPublicKeyBytes(decodedAddress)

		nonce, err := uuid.Parse(nonceString)
		if err != nil {
			return nil, nil, errTwitterInvalidRegistrationValue
		}

		decodedSignature, err := base58.Decode(signatureString)
		if err != nil {
			return nil, nil, errTwitterInvalidRegistrationValue
		}
		if len(decodedSignature) != 64 {
			return nil, nil, errTwitterInvalidRegistrationValue
		}

		// Validate the components of the registration value

		var tipAuthority *common.Account
		accountInfoRecord, err := p.data.GetAccountInfoByTokenAddress(ctx, tipAccount.PublicKey().ToBase58())
		switch err {
		case nil:
			if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
				return nil, nil, errTwitterInvalidRegistrationValue
			}

			tipAuthority, err = common.NewAccountFromPublicKeyString(accountInfoRecord.AuthorityAccount)
			if err != nil {
				return nil, nil, errors.Wrap(err, "invalid tip authority account")
			}
		case account.ErrAccountInfoNotFound:
			return nil, nil, errTwitterInvalidRegistrationValue
		default:
			return nil, nil, errors.Wrap(err, "error getting account info")
		}

		if !ed25519.Verify(tipAuthority.PublicKey().ToBytes(), nonce[:], decodedSignature) {
			return nil, nil, errTwitterInvalidRegistrationValue
		}

		return tipAccount, &nonce, nil
	}

	return nil, nil, errTwitterRegistrationNotFound
}

func toProtoVerifiedType(value string) userpb.TwitterUser_VerifiedType {
	switch value {
	case "blue":
		return userpb.TwitterUser_BLUE
	case "business":
		return userpb.TwitterUser_BUSINESS
	case "government":
		return userpb.TwitterUser_GOVERNMENT
	default:
		return userpb.TwitterUser_NONE
	}
}
