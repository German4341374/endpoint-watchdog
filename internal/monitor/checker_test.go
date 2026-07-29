package monitor_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
	"github.com/German4341374/endpoint-watchdog/internal/monitor"
)

func endpointFor(url string) config.Endpoint {
	return config.Endpoint{
		Name:           "test",
		URL:            url,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
		Timeout:        config.Duration{Duration: 250 * time.Millisecond},
		Interval:       config.Duration{Duration: time.Second},
	}
}

func checker(attempts int, threshold time.Duration) *monitor.Checker {
	return monitor.NewChecker(
		&http.Client{},
		4,
		config.Retry{
			Attempts:       attempts,
			InitialBackoff: config.Duration{Duration: time.Millisecond},
		},
		threshold,
	)
}

func TestCheckerReturnsUpForExpectedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Probe") != "present" {
			t.Error("expected configured header")
		}
		_, _ = io.WriteString(writer, "service ready")
	}))
	t.Cleanup(server.Close)

	endpoint := endpointFor(server.URL)
	endpoint.ExpectedText = "ready"
	endpoint.Headers = map[string]string{"X-Probe": "present"}
	result := checker(1, time.Second).Check(context.Background(), endpoint)

	if result.State != monitor.StateUp || result.StatusCode != http.StatusOK {
		t.Fatalf("Check() = %+v, want UP", result)
	}
}

func TestCheckerReturnsDownForUnexpectedStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	result := checker(1, time.Second).Check(context.Background(), endpointFor(server.URL))

	if result.State != monitor.StateDown || result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Check() = %+v, want DOWN", result)
	}
	if !strings.Contains(result.Error, "expected HTTP 200") {
		t.Fatalf("error = %q, want status mismatch", result.Error)
	}
}

func TestCheckerReturnsDownWhenExpectedTextIsMissing(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "starting")
	}))
	t.Cleanup(server.Close)

	endpoint := endpointFor(server.URL)
	endpoint.ExpectedText = "ready"
	result := checker(1, time.Second).Check(context.Background(), endpoint)

	if result.State != monitor.StateDown || !strings.Contains(result.Error, "was not found") {
		t.Fatalf("Check() = %+v, want missing text failure", result)
	}
}

func TestCheckerReturnsDegradedForSlowSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	result := checker(1, 5*time.Millisecond).Check(context.Background(), endpointFor(server.URL))

	if result.State != monitor.StateDegraded {
		t.Fatalf("State = %s, want DEGRADED", result.State)
	}
	if result.ResponseTimeMS < 5 {
		t.Fatalf("ResponseTimeMS = %f, want at least 5", result.ResponseTimeMS)
	}
}

func TestCheckerReturnsDownOnTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	endpoint := endpointFor(server.URL)
	endpoint.Timeout.Duration = 5 * time.Millisecond
	result := checker(1, time.Second).Check(context.Background(), endpoint)

	if result.State != monitor.StateDown || !strings.Contains(result.Error, "timed out") {
		t.Fatalf("Check() = %+v, want timeout DOWN", result)
	}
}

func TestCheckerRetriesWithExponentialBackoff(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(writer, "retry", http.StatusBadGateway)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	result := checker(3, time.Second).Check(context.Background(), endpointFor(server.URL))

	if result.State != monitor.StateUp || result.Attempt != 3 || calls.Load() != 3 {
		t.Fatalf("Check() = %+v, calls = %d, want third-attempt UP", result, calls.Load())
	}
}

type concurrencyDoer struct {
	active  atomic.Int32
	maximum atomic.Int32
	release <-chan struct{}
}

func (doer *concurrencyDoer) Do(_ *http.Request) (*http.Response, error) {
	active := doer.active.Add(1)
	for {
		maximum := doer.maximum.Load()
		if active <= maximum || doer.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	<-doer.release
	doer.active.Add(-1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
	}, nil
}

func TestCheckerLimitsConcurrentRequests(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	doer := &concurrencyDoer{release: release}
	limitedChecker := monitor.NewChecker(
		doer,
		2,
		config.Retry{Attempts: 1, InitialBackoff: config.Duration{Duration: time.Millisecond}},
		time.Second,
	)
	endpoint := endpointFor("http://example.test")

	var waitGroup sync.WaitGroup
	for range 6 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			limitedChecker.Check(context.Background(), endpoint)
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	waitGroup.Wait()

	if maximum := doer.maximum.Load(); maximum != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", maximum)
	}
}

func TestCheckerHonorsCancelledContext(t *testing.T) {
	t.Parallel()
	contextValue, cancel := context.WithCancel(context.Background())
	cancel()

	result := checker(2, time.Second).Check(contextValue, endpointFor("http://example.test"))

	if result.State != monitor.StateDown {
		t.Fatalf("State = %s, want DOWN", result.State)
	}
	if !strings.Contains(result.Error, "cancel") {
		t.Fatalf("error = %q, want cancellation", result.Error)
	}
}
