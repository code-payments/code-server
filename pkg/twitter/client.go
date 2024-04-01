package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	baseUrl = "https://api.twitter.com/2/"

	bearerTokenMaxAge = 15 * time.Minute

	metricsStructName = "twitter.client"
)

type Client struct {
	httpClient *http.Client

	clientId     string
	clientSecret string

	bearerTokenMu          sync.RWMutex
	bearerToken            string
	lastBearerTokenRefresh time.Time
}

// NewClient returns a new Twitter client
func NewClient(clientId, clientSecret string) *Client {
	return &Client{
		httpClient:   http.DefaultClient,
		clientId:     clientId,
		clientSecret: clientSecret,
	}
}

// User represents the structure for a user in the Twitter API response
type User struct {
	ID              string        `json:"id"`
	Username        string        `json:"username"`
	Name            string        `json:"name"`
	VerifiedType    string        `json:"verified_type"`
	ProfileImageUrl string        `json:"profile_image_url"`
	PublicMetrics   PublicMetrics `json:"public_metrics"`
}

// PublicMetrics represents the structure for public metrics in the Twitter API response
type PublicMetrics struct {
	FollowersCount int `json:"followers_count"`
	FollowingCount int `json:"following_count"`
	TweetCount     int `json:"tweet_count"`
	LikeCount      int `json:"like_count"`
}

// Tweet represents the structure for a tweet in the Twitter API response
type Tweet struct {
	ID       string  `json:"id"`
	Text     string  `json:"text"`
	AuthorID *string `json:"author_id"`

	AdditionalMetadata AdditionalTweetMetadata
}

// AdditionalTweetMetadata adds additinal metadata to a tweet that isn't directly
// represented in the Twitter API response
type AdditionalTweetMetadata struct {
	Author *User
}

// GetUserById makes a request to the Twitter API and returns the user's information
// by ID
func (c *Client) GetUserById(ctx context.Context, id string) (*User, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUserById")
	defer tracer.End()

	user, err := c.getUser(ctx, baseUrl+"users/"+id)
	if err != nil {
		tracer.OnError(err)
	}
	return user, err
}

// GetUserByUsername makes a request to the Twitter API and returns the user's information
// by username
func (c *Client) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUserByUsername")
	defer tracer.End()

	user, err := c.getUser(ctx, baseUrl+"users/by/username/"+username)
	if err != nil {
		tracer.OnError(err)
	}
	return user, err
}

// GetUserTweets gets tweets for a given user
func (c *Client) GetUserTweets(ctx context.Context, userId string, maxResults int, nextToken *string) ([]*Tweet, *string, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUserTweets")
	defer tracer.End()

	url := fmt.Sprintf(baseUrl+"users/"+userId+"/tweets?max_results=%d", maxResults)
	if nextToken != nil {
		url = fmt.Sprintf("%s&next_token=%s", url, *nextToken)
	}

	tweets, nextToken, err := c.getTweets(ctx, url)
	if err != nil {
		tracer.OnError(err)
	}
	return tweets, nextToken, err
}

// SearchRecentTweets searches for recent tweets within the last 7 days matching
// a search string.
func (c *Client) SearchRecentTweets(ctx context.Context, searchString string, maxResults int, nextToken *string) ([]*Tweet, *string, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "SearchUserTweets")
	defer tracer.End()

	url := fmt.Sprintf(
		baseUrl+"tweets/search/recent?query=%s&expansions=author_id&user.fields=username&max_results=%d",
		url.QueryEscape(searchString),
		maxResults,
	)
	if nextToken != nil {
		url = fmt.Sprintf("%s&next_token=%s", url, *nextToken)
	}

	tweets, nextToken, err := c.getTweets(ctx, url)
	if err != nil {
		tracer.OnError(err)
	}
	return tweets, nextToken, err
}

func (c *Client) getUser(ctx context.Context, fromUrl string) (*User, error) {
	bearerToken, err := c.getBearerToken(c.clientId, c.clientSecret)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", fromUrl+"?user.fields=profile_image_url,public_metrics,verified_type", nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status code: %d", resp.StatusCode)
	}

	var result struct {
		Data   *User           `json:"data"`
		Errors []*twitterError `json:"errors"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if len(result.Errors) > 0 {
		return nil, result.Errors[0].toError()
	}
	return result.Data, nil
}

func (c *Client) getTweets(ctx context.Context, fromUrl string) ([]*Tweet, *string, error) {
	bearerToken, err := c.getBearerToken(c.clientId, c.clientSecret)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", fromUrl, nil)
	if err != nil {
		return nil, nil, err
	}

	req = req.WithContext(ctx)

	req.Header.Add("Authorization", "Bearer "+bearerToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected http status code: %d", resp.StatusCode)
	}

	var result struct {
		Data   []*Tweet        `json:"data"`
		Errors []*twitterError `json:"errors"`
		Meta   struct {
			NextToken *string `json:"next_token"`
		} `json:"meta"`
		Includes struct {
			Users []User `json:"users"`
		} `json:"includes"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, nil, err
	}

	if len(result.Errors) > 0 {
		return nil, nil, result.Errors[0].toError()
	}

	for _, tweet := range result.Data {
		if tweet.AuthorID == nil {
			continue
		}

		for _, user := range result.Includes.Users {
			if user.ID == *tweet.AuthorID {
				tweet.AdditionalMetadata.Author = &user
				break
			}
		}
	}

	return result.Data, result.Meta.NextToken, nil
}

func (c *Client) getBearerToken(clientId, clientSecret string) (string, error) {
	c.bearerTokenMu.RLock()
	if time.Since(c.lastBearerTokenRefresh) < bearerTokenMaxAge {
		c.bearerTokenMu.RUnlock()
		return c.bearerToken, nil
	}
	c.bearerTokenMu.RUnlock()

	c.bearerTokenMu.Lock()
	defer c.bearerTokenMu.Unlock()

	if time.Since(c.lastBearerTokenRefresh) < bearerTokenMaxAge {
		return c.bearerToken, nil
	}

	requestData := []byte("grant_type=client_credentials")
	req, err := http.NewRequest("POST", "https://api.twitter.com/oauth2/token", bytes.NewBuffer(requestData))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientId, clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		TokenType   string `json:"token_type"`
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.AccessToken) == 0 {
		return "", fmt.Errorf("could not get access token")
	}

	c.bearerToken = result.AccessToken
	c.lastBearerTokenRefresh = time.Now()

	return result.AccessToken, nil
}

type twitterError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (e *twitterError) toError() error {
	return errors.Errorf("%s: %s", e.Title, e.Detail)
}
