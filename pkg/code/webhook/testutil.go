package webhook

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/code-payments/code-server/pkg/netutil"
	"github.com/code-payments/code-server/pkg/pointer"
	"github.com/code-payments/code-server/pkg/testutil"
	"github.com/code-payments/code-server/pkg/code/data/webhook"
)

type TestWebhookEndpoint struct {
	mu          sync.Mutex
	port        int32
	requests    []string
	shouldError bool
	delay       time.Duration
}

// NewTestWebhookEndpoint returns a new server for testing webhook execution
func NewTestWebhookEndpoint(t *testing.T) *TestWebhookEndpoint {
	availablePort, err := netutil.GetAvailablePortForAddress("localhost")
	require.NoError(t, err)

	server := &TestWebhookEndpoint{
		port: availablePort,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", server.handler)
	go func() {
		require.NoError(t, http.ListenAndServe(fmt.Sprintf(":%d", availablePort), mux))
	}()
	return server
}

func (s *TestWebhookEndpoint) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if r.Header.Get(contentTypeHeaderName) != contentTypeHeaderValue {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	shouldError := s.shouldError
	delay := s.delay
	s.requests = append(s.requests, string(body))
	s.mu.Unlock()

	time.Sleep(delay)

	if shouldError {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *TestWebhookEndpoint) GetReceivedRequests() []string {
	s.mu.Lock()
	copied := make([]string, len(s.requests))
	copy(copied, s.requests)
	s.mu.Unlock()
	return copied
}

func (s *TestWebhookEndpoint) GetRandomWebhookRecord(t *testing.T, webhookType webhook.Type) *webhook.Record {
	return &webhook.Record{
		WebhookId:     testutil.NewRandomAccount(t).PublicKey().ToBase58(),
		Url:           fmt.Sprintf("http://localhost:%d/webhook", s.port),
		Type:          webhookType,
		State:         webhook.StatePending,
		NextAttemptAt: pointer.Time(time.Now()),
	}
}

func (s *TestWebhookEndpoint) SimulateErrors() {
	s.mu.Lock()
	s.shouldError = true
	s.mu.Unlock()
}

func (s *TestWebhookEndpoint) SimulateDelay(delay time.Duration) {
	s.mu.Lock()
	s.delay = delay
	s.mu.Unlock()
}

func (s *TestWebhookEndpoint) Reset() {
	s.mu.Lock()
	s.shouldError = false
	s.delay = 0
	s.requests = nil
	s.mu.Unlock()
}
