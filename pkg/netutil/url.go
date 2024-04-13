package netutil

import (
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/retry"
	"github.com/code-payments/code-server/pkg/retry/backoff"
)

// ValidateHttpUrl validates a URL for an HTTP scheme
func ValidateHttpUrl(
	value string,
	requireSecureConnection bool,
	fetchContent bool,
) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}

	if len(parsed.Scheme) == 0 {
		// Add a HTTP scheme by default
		value = "http://" + value
		parsed, err = url.Parse(value)
		if err != nil {
			return err
		}
	}

	if requireSecureConnection && parsed.Scheme != "https" {
		return errors.New("url scheme must be https")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url scheme must be http or https")
	}

	if len(parsed.Host) == 0 {
		return errors.New("host component missing")
	} else if err := ValidateDomainName(parsed.Host); err != nil {
		return errors.Wrap(err, "host is not a valid domain name")
	}

	if fetchContent {
		// Best-effort attempt to fetch the content
		var resp *http.Response
		_, err = retry.Retry(
			func() error {
				// Retry only occurs if err != nil, in which case the body does not need to be closed.
				// The body itself is closed below
				resp, err = http.Get(value) //nolint:bodyclose
				return err
			},
			retry.Limit(5),
			retry.Backoff(backoff.BinaryExponential(100*time.Millisecond), time.Second),
		)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errors.Errorf("%d status code fetching content", resp.StatusCode)
		}
	} else {
		// If not fetching content, then ensure the hostname is valid
		_, err := net.LookupIP(parsed.Hostname())
		if err != nil {
			return errors.Wrap(err, "error resolving hostname")
		}
	}

	return nil
}
