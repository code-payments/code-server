package coingecko

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	metricsStructName = "currency.coingecko.client"
)

const (
	baseUrl             = "https://api.coingecko.com/api"
	latestUrlFormat     = baseUrl + "/v3/coins/%s?localization=false&tickers=false&community_data=false&developer_data=false&sparkline=false"
	historicalUrlFormat = baseUrl + "/v3/coins/%s/history?date=%s&localization=false"
)

type client struct {
	httpClient *http.Client
	retrier    retry.Retrier
}

func NewClient() currency.Client {
	return &client{
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
	err := c.submitRequest(ctx, url, http.NoBody, &resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	return resp.toExchangeData(), nil
}

// GetHistoricalRates implements currency.Client.GetHistoricalRates
func (c *client) GetHistoricalRates(ctx context.Context, base string, timestamp time.Time) (*currency.ExchangeData, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetHistoricalRates")
	defer tracer.End()

	var resp response
	url := fmt.Sprintf(historicalUrlFormat, base, timestamp.Format("02-01-2006"))
	err := c.submitRequest(ctx, url, http.NoBody, &resp)
	if err != nil {
		tracer.OnError(err)
		return nil, err
	}

	resp.LastUpdated = time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())

	return resp.toExchangeData(), nil
}

func (c *client) submitRequest(ctx context.Context, url string, body io.Reader, resp interface{}) error {
	// todo: add rate limiting
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	var httpResp *http.Response
	_, err = c.retrier.Retry(
		func() error {
			// Retry only occurs if err != nil, in which case the body does not need to be closed.
			// The body itself is closed below
			httpResp, err = c.httpClient.Do(req) //nolint:bodyclose
			return err
		},
	)
	if err != nil {
		return errors.Wrap(err, "failed to make request")
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode == http.StatusNotFound {
		return currency.ErrInvalidBase
	} else if httpResp.StatusCode != http.StatusOK {
		return errors.Errorf("received non-200 status code: %d", httpResp.StatusCode)
	}

	err = json.NewDecoder(httpResp.Body).Decode(resp)
	if err != nil {
		return errors.Wrap(err, "failed to decode response")
	}

	return nil
}

type response struct {
	Symbol     string `json:"symbol"`
	MarketData struct {
		CurrentPrice map[string]float64 `json:"current_price"`
	} `json:"market_data"`
	LastUpdated time.Time `json:"last_updated"`
}

func (r response) toExchangeData() *currency.ExchangeData {
	rates := make(map[string]float64)
	for symbol, rate := range r.MarketData.CurrentPrice {
		rates[strings.ToLower(symbol)] = rate
	}
	rates = excludeUnsupported(rates)

	return &currency.ExchangeData{
		Base:      strings.ToLower(r.Symbol),
		Rates:     rates,
		Timestamp: r.LastUpdated,
	}
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
