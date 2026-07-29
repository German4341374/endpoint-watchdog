package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
)

const maxExpectedTextBody = 1 << 20

// HTTPDoer is implemented by http.Client and enables deterministic tests.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Checker performs bounded HTTP attempts and exponential-backoff retries.
type Checker struct {
	client            HTTPDoer
	semaphore         chan struct{}
	retry             config.Retry
	degradedThreshold time.Duration
}

// NewChecker creates a checker with a global outbound concurrency limit.
func NewChecker(
	client HTTPDoer,
	maxConcurrent int,
	retry config.Retry,
	degradedThreshold time.Duration,
) *Checker {
	if client == nil {
		client = &http.Client{}
	}
	return &Checker{
		client:            client,
		semaphore:         make(chan struct{}, maxConcurrent),
		retry:             retry,
		degradedThreshold: degradedThreshold,
	}
}

// Check retries DOWN outcomes and returns only the final scheduled result.
func (checker *Checker) Check(ctx context.Context, endpoint config.Endpoint) Result {
	var result Result
	for attempt := 1; attempt <= checker.retry.Attempts; attempt++ {
		result = checker.checkOnce(ctx, endpoint)
		result.Attempt = attempt
		if result.State != StateDown || attempt == checker.retry.Attempts || ctx.Err() != nil {
			return result
		}
		delay := checker.retry.InitialBackoff.Duration * time.Duration(1<<uint(attempt-1))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			result.Error = "check cancelled during retry backoff"
			return result
		case <-timer.C:
		}
	}
	return result
}

func (checker *Checker) checkOnce(ctx context.Context, endpoint config.Endpoint) Result {
	if err := checker.acquire(ctx); err != nil {
		return downResult(0, err)
	}
	defer checker.release()

	requestContext, cancel := context.WithTimeout(ctx, endpoint.Timeout.Duration)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, endpoint.Method, endpoint.URL, nil)
	if err != nil {
		return downResult(0, fmt.Errorf("create request: %w", err))
	}
	for name, value := range endpoint.Headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("User-Agent", "endpoint-watchdog/1.0")

	started := time.Now()
	response, err := checker.client.Do(request)
	if err != nil {
		return downResult(time.Since(started), classifyRequestError(err))
	}
	defer response.Body.Close()

	body, readErr := readResponseBody(response.Body, endpoint.ExpectedText != "")
	elapsed := time.Since(started)
	if readErr != nil {
		return downHTTPResult(elapsed, response.StatusCode, fmt.Errorf("read response: %w", readErr))
	}
	if response.StatusCode != endpoint.ExpectedStatus {
		return downHTTPResult(
			elapsed,
			response.StatusCode,
			fmt.Errorf("expected HTTP %d, received %d", endpoint.ExpectedStatus, response.StatusCode),
		)
	}
	if endpoint.ExpectedText != "" && !bytes.Contains(body, []byte(endpoint.ExpectedText)) {
		return downHTTPResult(
			elapsed,
			response.StatusCode,
			fmt.Errorf("expected text %q was not found", endpoint.ExpectedText),
		)
	}

	state := StateUp
	if elapsed > checker.degradedThreshold {
		state = StateDegraded
	}
	return Result{
		State:          state,
		CheckedAt:      time.Now().UTC(),
		ResponseTimeMS: durationMilliseconds(elapsed),
		StatusCode:     response.StatusCode,
	}
}

func (checker *Checker) acquire(ctx context.Context) error {
	select {
	case checker.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (checker *Checker) release() {
	<-checker.semaphore
}

func readResponseBody(body io.Reader, expectedText bool) ([]byte, error) {
	if expectedText {
		return io.ReadAll(io.LimitReader(body, maxExpectedTextBody))
	}
	_, err := io.Copy(io.Discard, io.LimitReader(body, 64*1024))
	return nil, err
}

func classifyRequestError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("request timed out")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("request cancelled")
	}
	return fmt.Errorf("request failed: %w", err)
}

func downResult(elapsed time.Duration, err error) Result {
	return downHTTPResult(elapsed, 0, err)
}

func downHTTPResult(elapsed time.Duration, statusCode int, err error) Result {
	return Result{
		State:          StateDown,
		CheckedAt:      time.Now().UTC(),
		ResponseTimeMS: durationMilliseconds(elapsed),
		StatusCode:     statusCode,
		Error:          err.Error(),
	}
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
