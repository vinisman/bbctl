package bitbucket

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/vinisman/bbctl/internal/config"
)

// retryTransport is an http.RoundTripper with retry support.
//
// Retried:
//   - any request on 429 (rate limit) and 502/503/504 (server unavailable) —
//     the server did not accept or process the request, a retry is safe;
//   - idempotent requests (GET/PUT/DELETE) on 5xx and network errors;
//   - POST on network errors is NOT retried (not idempotent).
//
// Backoff: exponential with base delay and full jitter; 429 honors
// Retry-After when it is larger than the computed delay.
//
// Configuration (env, read in internal/config):
//   - BITBUCKET_RETRY_MAX_ATTEMPTS  — total attempts, including the first one (default 4)
//   - BITBUCKET_RETRY_BASE_DELAY_MS — base backoff delay, ms (default 500)
type retryTransport struct {
	base http.RoundTripper
}

// retryableStatus returns true if the request should be retried for any method.
func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// idempotentMethod returns true if a retry is safe for any 5xx/network error.
func idempotentMethod(method string) bool {
	return method == http.MethodGet ||
		method == http.MethodPut ||
		method == http.MethodDelete ||
		method == http.MethodHead
}

// bufferBody reads the request body so it can be replayed across retries.
func bufferBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	return body, nil
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	maxAttempts := config.GlobalRetryMaxAttempts
	if maxAttempts < 1 {
		// Guard against uninitialized globals: otherwise the loop never runs
		// and RoundTrip returns (nil, nil) — silently breaking every request.
		maxAttempts = 1
	}
	baseDelay := time.Duration(config.GlobalRetryBaseDelayMs) * time.Millisecond
	logger := config.GlobalLogger

	var body []byte
	if req.Body != nil {
		var err error
		if body, err = bufferBody(req); err != nil {
			return nil, fmt.Errorf("retry: failed to buffer request body: %w", err)
		}
	}

	backoff := baseDelay
	var lastErr error
	var lastResp *http.Response

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Replay the body on every attempt
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}

		resp, err := t.base.RoundTrip(req)

		// Context canceled/expired — retrying is pointless
		if req.Context().Err() != nil {
			return resp, err
		}

		// Network error: retry only idempotent methods
		if err != nil {
			lastErr = err
			lastResp = nil
			if !idempotentMethod(req.Method) {
				return nil, err
			}
		} else {
			lastResp = resp
			lastErr = nil

			// Success — hand it up
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return resp, nil
			}

			// Retryable status: POST — only 429/502/503/504
			isRetryable := retryableStatus(resp.StatusCode) ||
				(idempotentMethod(req.Method) && resp.StatusCode >= 500)
			if !isRetryable {
				return resp, nil
			}

			// Drain and close the error response body to free the connection
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()

			lastErr = fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		}

		if attempt == maxAttempts {
			break
		}

		// Delay: exponential backoff + full jitter;
		// 429 honors Retry-After when it is larger
		delay := time.Duration(rand.Int63n(int64(backoff)+1)) + backoff/2
		if lastResp != nil && lastResp.StatusCode == http.StatusTooManyRequests {
			if ra := lastResp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					retryAfter := time.Duration(secs) * time.Second
					if retryAfter > delay {
						delay = retryAfter
					}
				}
			}
		}

		logger.Debug("Retrying request after error",
			"method", req.Method,
			"url", req.URL.String(),
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"delay_ms", delay.Milliseconds(),
			"error", lastErr)

		select {
		case <-time.After(delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}

		backoff *= 2
	}

	if lastErr != nil {
		return nil, fmt.Errorf("retry: %d attempts exhausted: %w", maxAttempts, lastErr)
	}
	return lastResp, nil
}

// applyRetryTransport wraps the http client transport with the retry layer.
func applyRetryTransport(client *http.Client, logger *slog.Logger) {
	logger.Debug("Enabling HTTP retry transport",
		"max_attempts", config.GlobalRetryMaxAttempts,
		"base_delay_ms", config.GlobalRetryBaseDelayMs)
	base := client.Transport
	if base == nil {
		// Without an explicit transport (no --insecure) the client uses DefaultTransport
		base = http.DefaultTransport
	}
	client.Transport = &retryTransport{base: base}
}
