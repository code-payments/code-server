package fixer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/currency"
	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

const (
	metricsStructName = "currency.fixer.client"
)

const (
	baseUrl             = "https://api.apilayer.com/fixer"
	latestUrlFormat     = baseUrl + "/latest?base=%s"
	historicalUrlFormat = baseUrl + "/%s?base=%s"

	apiKeyHeaderName = "apikey"
)

const (
	invalidBaseErrorCode = 201
)

// API Documentation: https://apilayer.com/marketplace/fixer-api#documentation-tab
type client struct {
	apiKey     string
	httpClient *http.Client
	retrier    retry.Retrier
}

func NewClient(apiKey string) currency.Client {
	return &client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		retrier: retry.NewRetrier(
			retry.NonRetriableErrors(context.Canceled),
			retry.Limit(3),
			retry.BackoffWithJitter(backoff.BinaryExponential(time.Second), 10*time.Second, 0.1),
		),
	}
}

// GetCurrentRates implements currency.Client.GetCurrentRates
func (c *client) GetCurrentRates(ctx context.Context, base string) (*currency.ExchangeData, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetCurrentRates")
	defer tracer.End()

	var resp response
	url := fmt.Sprintf(latestUrlFormat, base)
	err := c.submitRequest(ctx, url, &resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	err = checkCustomError(resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	return resp.toExchangeData()
}

// GetHistoricalRates implements currency.Client.GetHistoricalRates
func (c *client) GetHistoricalRates(ctx context.Context, base string, timestamp time.Time) (*currency.ExchangeData, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetHistoricalRates")
	defer tracer.End()

	var resp response
	url := fmt.Sprintf(historicalUrlFormat, timestamp.Format("2006-01-02"), base)
	err := c.submitRequest(ctx, url, &resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	err = checkCustomError(resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	truncatedTimestamp := time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())
	resp.Timestamp = uint64(truncatedTimestamp.Unix())

	return resp.toExchangeData()
}

func (c *client) submitRequest(ctx context.Context, url string, resp interface{}) error {
	// todo: add rate limiting

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	req.Header.Set(apiKeyHeaderName, c.apiKey)

	var httpResp *http.Response
	_, err = c.retrier.Retry(
		func() error {
			httpResp, err = c.httpClient.Do(req)
			return err
		},
	)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return errors.Errorf("received non-200 status code: %d", httpResp.StatusCode)
	}

	err = json.NewDecoder(httpResp.Body).Decode(resp)
	if err != nil {
		return errors.Wrap(err, "failed to decode response")
	}

	return nil
}

func checkCustomError(resp response) error {
	if resp.Success {
		return nil
	}

	if resp.Error == nil {
		return errors.New("unknown error from fixer without code")
	}

	if resp.Error.Code == invalidBaseErrorCode {
		return currency.ErrInvalidBase
	}

	return errors.Errorf("fixer error %d: %s", resp.Error.Code, resp.Error.Info)
}

type response struct {
	Success bool `json:"success"`
	Error   *struct {
		Code int    `json:"code"`
		Info string `json:"info"`
	} `json:"error"`

	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
	Timestamp uint64             `json:"timestamp"`
}

func (r response) toExchangeData() (*currency.ExchangeData, error) {
	if !r.Success {
		return nil, errors.New("cannot convert a failed response")
	}

	rates := make(map[string]float64)
	for symbol, rate := range r.Rates {
		rates[strings.ToLower(symbol)] = rate
	}
	rates = excludeUnsupported(rates)

	return &currency.ExchangeData{
		Base:      strings.ToLower(r.Base),
		Rates:     rates,
		Timestamp: time.Unix(int64(r.Timestamp), 0),
	}, nil
}

func excludeUnsupported(data map[string]float64) map[string]float64 {
	res := make(map[string]float64, 0)

	for symbol, rate := range data {
		if len(symbol) == 3 {
			res[symbol] = rate
		}
	}

	return res
}
