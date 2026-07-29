package web_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
	"github.com/German4341374/endpoint-watchdog/internal/monitor"
	"github.com/German4341374/endpoint-watchdog/internal/web"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	endpoint := config.Endpoint{
		Name:           "Public API",
		URL:            "https://api.example.test/health?api_key=hidden",
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
	}
	store := monitor.NewStore([]config.Endpoint{endpoint})
	store.Record(endpoint.Name, monitor.Result{
		State:          monitor.StateUp,
		CheckedAt:      time.Date(2026, 7, 29, 9, 30, 0, 0, time.UTC),
		ResponseTimeMS: 18.4,
		StatusCode:     http.StatusOK,
		Attempt:        1,
	})
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	server, err := web.New("Support Platform", store, logger)
	if err != nil {
		t.Fatalf("web.New() error = %v", err)
	}
	return server.Handler()
}

func TestHealthHandler(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status ok", body)
	}
}

func TestStatusHandlerReturnsSummary(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	var summary monitor.Summary
	if err := json.Unmarshal(recorder.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Service != "Support Platform" || summary.State != monitor.StateUp {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.Endpoints) != 1 || strings.Contains(summary.Endpoints[0].URL, "api_key") {
		t.Fatalf("unexpected endpoint response: %+v", summary.Endpoints)
	}
}

func TestEndpointStatusIncludesHistory(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/api/status/Public%20API", nil)
	request.SetPathValue("name", "Public API")
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var status monitor.EndpointStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode endpoint: %v", err)
	}
	if len(status.History) != 1 || status.History[0].State != monitor.StateUp {
		t.Fatalf("status = %+v, want history", status)
	}
}

func TestEndpointStatusReturnsStructuredNotFound(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/api/status/Missing", nil)
	request.SetPathValue("name", "Missing")
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("ENDPOINT_NOT_FOUND")) {
		t.Fatalf("body = %s, want error code", recorder.Body.String())
	}
}

func TestStatusPageRendersCurrentState(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	body := recorder.Body.String()
	for _, expected := range []string{"Support Platform", "Public API", "UP", "18.4 ms", "100.00%"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body does not contain %q", expected)
		}
	}
}

func TestUnknownPageReturnsNotFound(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestHandlersSetSecurityHeaders(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing X-Frame-Options")
	}
	if recorder.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing Content-Security-Policy")
	}
}

func TestUnsupportedMethodReturnsMethodNotAllowed(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	recorder := httptest.NewRecorder()

	testHandler(t).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
