package bitbucket

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vinisman/bbctl/internal/config"
)

// retryTestSetup applies fast retry settings and silences the logger.
func retryTestSetup(t *testing.T, maxAttempts, baseDelayMs int) {
	t.Helper()
	t.Cleanup(func() {
		config.GlobalRetryMaxAttempts = 4
		config.GlobalRetryBaseDelayMs = 500
	})
	config.GlobalRetryMaxAttempts = maxAttempts
	config.GlobalRetryBaseDelayMs = baseDelayMs
	config.GlobalLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newRetryClient() *http.Client {
	return &http.Client{Transport: &retryTransport{base: http.DefaultTransport}}
}

// GET: 500, 500, then 200 — three attempts, success.
func TestRetryTransport_GetRetriesOn500(t *testing.T) {
	retryTestSetup(t, 4, 1)

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer ts.Close()

	resp, err := newRetryClient().Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

// POST: 500 — exactly one attempt (non-idempotent methods are not retried).
func TestRetryTransport_PostNotRetriedOn500(t *testing.T) {
	retryTestSetup(t, 4, 1)

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	resp, err := newRetryClient().Post(ts.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (POST must not be retried)", got)
	}
}

// 429: any method is retried, Retry-After is honored.
func TestRetryTransport_429HonorsRetryAfter(t *testing.T) {
	retryTestSetup(t, 3, 1)

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer ts.Close()

	start := time.Now()
	resp, err := newRetryClient().Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("Retry-After=1s ignored: elapsed %v", elapsed)
	}
}

// Network error (server unreachable): GET — all attempts exhausted,
// the error carries the exhaustion marker.
func TestRetryTransport_NetworkErrorExhaustsRetries(t *testing.T) {
	retryTestSetup(t, 3, 1)

	// URL of a closed server: the connection is rejected
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := ts.URL
	ts.Close() // port freed — the connection fails

	_, err := newRetryClient().Get(baseURL)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "attempts exhausted") {
		t.Fatalf("error should mark retry exhaustion, got: %v", err)
	}
}

// The request body is replayed on every attempt.
func TestRetryTransport_BodyReplayedOnRetry(t *testing.T) {
	retryTestSetup(t, 3, 1)

	var mu sync.Mutex
	var attempts int32
	bodies := map[int]string{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := int(atomic.AddInt32(&attempts, 1))

		mu.Lock()
		bodies[n] = string(body)
		mu.Unlock()

		if n < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`ok`))
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPut, ts.URL, strings.NewReader(`{"k":"v"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := newRetryClient().Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	for i := 1; i <= 2; i++ {
		if bodies[i] != `{"k":"v"}` {
			t.Fatalf("attempt %d: body = %q, want {\"k\":\"v\"}", i, bodies[i])
		}
	}
}

// A successful response is not retried.
func TestRetryTransport_SuccessNoRetry(t *testing.T) {
	retryTestSetup(t, 4, 1)

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer ts.Close()

	resp, err := newRetryClient().Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

// Uninitialized retry globals must not break a plain request
// (before: maxAttempts=0 → the loop never ran → RoundTrip returned (nil, nil)).
func TestRetryTransport_UninitializedGlobals(t *testing.T) {
	retryTestSetup(t, 4, 1)
	// simulate zeroed globals (e.g. before LoadConfig)
	config.GlobalRetryMaxAttempts = 0
	config.GlobalRetryBaseDelayMs = 0

	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer ts.Close()

	resp, err := newRetryClient().Get(ts.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %v, want 200", resp)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

// applyRetryTransport installs a fallback transport (protection against nil-base panic).
func TestApplyRetryTransport_NilBaseFallback(t *testing.T) {
	retryTestSetup(t, 2, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	client := &http.Client{} // Transport == nil
	applyRetryTransport(client, logger)

	tr, ok := client.Transport.(*retryTransport)
	if !ok {
		t.Fatalf("transport not wrapped, got %T", client.Transport)
	}
	if tr.base == nil {
		t.Fatal("retryTransport.base is nil — RoundTrip will panic")
	}
	if tr.base != http.DefaultTransport {
		t.Fatalf("base = %T, want http.DefaultTransport", tr.base)
	}
}

// Context cancellation stops retries.
func TestRetryTransport_ContextCancelStopsRetries(t *testing.T) {
	retryTestSetup(t, 5, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	start := time.Now()
	_, err := newRetryClient().Do(req)
	if err == nil {
		t.Fatal("expected error after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("cancellation not honored, elapsed %v", elapsed)
	}
}
