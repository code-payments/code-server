package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/metrics"
)

const (
	baseUrl              = "https://api.twitter.com/2/"
	getUserByIdUrl       = baseUrl + "users/"
	getUserByUsernameUrl = baseUrl + "users/by/username/"

	metricsStructName = "twitter.client"
)

type Client struct {
	client   *http.Client
	apiToken string
}

// NewClient returns a new Twitter client
func NewClient(apiToken string) *Client {
	return &Client{
		client:   http.DefaultClient,
		apiToken: apiToken,
	}
}

// User represents the structure for a user in the Twitter API response
type User struct {
	ID              string        `json:"id"`
	Username        string        `json:"username"`
	Name            string        `json:"name"`
	ProfileImageUrl string        `json:"profile_image_url"`
	PublicMetrics   PublicMetrics `json:"public_metrics"`
}

// PublicMetrics are public metrics for a Twitter user
type PublicMetrics struct {
	FollowersCount int `json:"followers_count"`
	FollowingCount int `json:"following_count"`
	TweetCount     int `json:"tweet_count"`
	LikeCount      int `json:"like_count"`
}

// GetUserById makes a request to the Twitter API and returns the user's information
// by ID
func (c *Client) GetUserById(ctx context.Context, id string) (*User, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetUserById")
	defer tracer.End()

	user, err := c.getUser(getUserByIdUrl + id)
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

	user, err := c.getUser(getUserByUsernameUrl + username)
	if err != nil {
		tracer.OnError(err)
	}
	return user, err
}

func (c *Client) getUser(fromUrl string) (*User, error) {
	req, err := http.NewRequest("GET", fromUrl+"?user.fields=profile_image_url,public_metrics", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected http status code: %d", resp.StatusCode)
	}

	var result struct {
		Data   User           `json:"data"`
		Errors []twitterError `json:"errors"`
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
	return &result.Data, nil
}

type twitterError struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func (e *twitterError) toError() error {
	return errors.Errorf("%s: %s", e.Title, e.Detail)
}
