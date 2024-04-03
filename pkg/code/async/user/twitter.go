package async_user

import (
	"context"
	"strings"
	"time"

	"github.com/mr-tron/base58"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/pkg/errors"

	commonpb "github.com/code-payments/code-protobuf-api/generated/go/common/v1"
	userpb "github.com/code-payments/code-protobuf-api/generated/go/user/v1"

	"github.com/code-payments/code-server/pkg/code/common"
	"github.com/code-payments/code-server/pkg/code/data/account"
	"github.com/code-payments/code-server/pkg/code/data/twitter"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	twitter_lib "github.com/code-payments/code-server/pkg/twitter"
)

const (
	tipCardRegistrationPrefix = "accountForX="
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

			err = p.findNewTwitterRegistrations(tracedCtx)
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

func (p *service) findNewTwitterRegistrations(ctx context.Context) error {
	var newlyProcessedTweets []string

	err := func() error {
		var pageToken *string
		processedUsernames := make(map[string]any)
		for {
			tweets, nextPageToken, err := p.twitterClient.SearchRecentTweets(
				ctx,
				tipCardRegistrationPrefix,
				maxTweetSearchResults,
				pageToken,
			)
			if err != nil {
				return errors.Wrap(err, "error searching tweets")
			}

			for _, tweet := range tweets {
				if tweet.AdditionalMetadata.Author == nil {
					return errors.Errorf("author missing in tweet %s", tweet.ID)
				}

				isTweetProcessed, err := p.data.IsTweetProcessed(ctx, tweet.ID)
				if err != nil {
					return errors.Wrap(err, "error checking if tweet is processed")
				} else if isTweetProcessed {
					// Found a checkpoint, so stop processing
					return nil
				}

				// Oldest tweets go first, so we are guaranteed to checkpoint everything
				newlyProcessedTweets = append([]string{tweet.ID}, newlyProcessedTweets...)

				// Avoid reprocessing a Twitter user and potentially overriding the
				// tip address with something older.
				if _, ok := processedUsernames[tweet.AdditionalMetadata.Author.Username]; ok {
					continue
				}

				// Attempt to find a tip account from the registration tweet
				tipAccount, err := findTipAccountRegisteredInTweet(tweet)
				switch err {
				case nil:
				case errTwitterInvalidRegistrationValue, errTwitterRegistrationNotFound:
					continue
				default:
					return errors.Wrapf(err, "unexpected error processing tweet %s", tweet.ID)
				}

				// Validate the new tip account
				accountInfoRecord, err := p.data.GetAccountInfoByTokenAddress(ctx, tipAccount.PublicKey().ToBase58())
				switch err {
				case nil:
					// todo: potentially use a relationship account instead
					if accountInfoRecord.AccountType != commonpb.AccountType_PRIMARY {
						continue
					}
				case account.ErrAccountInfoNotFound:
					continue
				default:
					return errors.Wrap(err, "error getting account info")
				}

				processedUsernames[tweet.AdditionalMetadata.Author.Username] = struct{}{}

				err = p.updateCachedTwitterUser(ctx, tweet.AdditionalMetadata.Author, tipAccount)
				if err != nil {
					return errors.Wrap(err, "error updating cached user state")
				}
			}

			if nextPageToken == nil {
				return nil
			}
			pageToken = nextPageToken
		}
	}()

	if err != nil {
		return err
	}

	// Only update the processed tweet cache once we've found another checkpoint,
	// or reached the end of the Tweet feed.
	//
	// todo: add batching
	for _, tweetId := range newlyProcessedTweets {
		err := p.data.MarkTweetAsProcessed(ctx, tweetId)
		if err != nil {
			return errors.Wrap(err, "error marking tweet as processed")
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

	record, err := p.data.GetTwitterUser(ctx, user.Username)
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
	if err != nil {
		return errors.Wrap(err, "error updating cached twitter user")
	}
	return nil
}

func findTipAccountRegisteredInTweet(tweet *twitter_lib.Tweet) (*common.Account, error) {
	var depositAccount *common.Account

	parts := strings.Fields(tweet.Text)
	for _, part := range parts {
		if !strings.HasPrefix(part, tipCardRegistrationPrefix) {
			continue
		}

		part = part[len(tipCardRegistrationPrefix):]
		part = strings.TrimSuffix(part, ".")

		decoded, err := base58.Decode(part)
		if err != nil {
			return nil, errTwitterInvalidRegistrationValue
		}

		if len(decoded) != 32 {
			return nil, errTwitterInvalidRegistrationValue
		}

		depositAccount, _ = common.NewAccountFromPublicKeyBytes(decoded)
		return depositAccount, nil
	}

	return nil, errTwitterRegistrationNotFound
}

func toProtoVerifiedType(value string) userpb.GetTwitterUserResponse_VerifiedType {
	switch value {
	case "blue":
		return userpb.GetTwitterUserResponse_BLUE
	case "business":
		return userpb.GetTwitterUserResponse_BUSINESS
	case "government":
		return userpb.GetTwitterUserResponse_GOVERNMENT
	default:
		return userpb.GetTwitterUserResponse_NONE
	}
}
