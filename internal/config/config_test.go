package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
)

const validYAML = `
server:
  address: ":9090"
  title: "Support Services"
  maxConcurrent: 3
  degradedThreshold: 400ms
retry:
  attempts: 2
  initialBackoff: 25ms
endpoints:
  - name: API
    url: https://api.example.test/health
    method: GET
    expectedStatus: 204
    timeout: 2s
    interval: 10s
    expectedText: ready
    headers:
      X-Monitor-Token: demo-placeholder
`

func TestDecodeValidConfiguration(t *testing.T) {
	t.Parallel()

	cfg, err := config.Decode(strings.NewReader(validYAML))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if cfg.Server.Address != ":9090" || cfg.Server.Title != "Support Services" {
		t.Fatalf("unexpected server config: %+v", cfg.Server)
	}
	if cfg.Server.MaxConcurrent != 3 {
		t.Fatalf("MaxConcurrent = %d, want 3", cfg.Server.MaxConcurrent)
	}
	endpoint := cfg.Endpoints[0]
	if endpoint.ExpectedStatus != 204 || endpoint.Timeout.Duration != 2*time.Second {
		t.Fatalf("unexpected endpoint defaults: %+v", endpoint)
	}
	if endpoint.Headers["X-Monitor-Token"] != "demo-placeholder" {
		t.Fatal("expected configured header")
	}
}

func TestDecodeAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := config.Decode(strings.NewReader(`
endpoints:
  - name: Local
    url: http://127.0.0.1:8080/health
`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	endpoint := cfg.Endpoints[0]
	if cfg.Server.MaxConcurrent != 5 {
		t.Fatalf("MaxConcurrent = %d, want 5", cfg.Server.MaxConcurrent)
	}
	if endpoint.Method != "GET" || endpoint.ExpectedStatus != 200 {
		t.Fatalf("unexpected endpoint defaults: %+v", endpoint)
	}
	if endpoint.Timeout.Duration != 5*time.Second || endpoint.Interval.Duration != 30*time.Second {
		t.Fatalf("unexpected duration defaults: %+v", endpoint)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
server:
  mysterySetting: true
endpoints:
  - name: API
    url: https://api.example.test
`))
	if err == nil || !strings.Contains(err.Error(), "field mysterySetting not found") {
		t.Fatalf("Decode() error = %v, want unknown field error", err)
	}
}

func TestDecodeRejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: File
    url: file:///etc/passwd
`))
	if err == nil || !strings.Contains(err.Error(), "absolute HTTP URL") {
		t.Fatalf("Decode() error = %v, want scheme error", err)
	}
}

func TestDecodeRejectsEmbeddedCredentials(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: Private
    url: https://user:password@example.test/health
`))
	if err == nil || !strings.Contains(err.Error(), "embedded credentials") {
		t.Fatalf("Decode() error = %v, want credentials error", err)
	}
}

func TestDecodeRejectsDuplicateNamesCaseInsensitively(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: API
    url: https://one.example.test
  - name: api
    url: https://two.example.test
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate endpoint name") {
		t.Fatalf("Decode() error = %v, want duplicate error", err)
	}
}

func TestDecodeRejectsUnsafeMethod(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: Mutating
    url: https://api.example.test/jobs
    method: POST
`))
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Decode() error = %v, want method error", err)
	}
}

func TestDecodeRejectsExpectedTextWithHead(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: HEAD
    url: https://api.example.test
    method: HEAD
    expectedText: ready
`))
	if err == nil || !strings.Contains(err.Error(), "cannot be used with HEAD") {
		t.Fatalf("Decode() error = %v, want expectedText error", err)
	}
}

func TestDecodeRejectsInvalidDuration(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: API
    url: https://api.example.test
    timeout: immediately
`))
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("Decode() error = %v, want duration error", err)
	}
}

func TestDecodeRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: API
    url: https://api.example.test
---
extra: document
`))
	if err == nil || !strings.Contains(err.Error(), "multiple YAML documents") {
		t.Fatalf("Decode() error = %v, want document error", err)
	}
}

func TestDecodeRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader(`
endpoints:
  - name: API
    url: https://api.example.test
    headers:
      "Bad Header": value
`))
	if err == nil || !strings.Contains(err.Error(), "invalid header") {
		t.Fatalf("Decode() error = %v, want header error", err)
	}
}

func TestDecodeRequiresEndpoints(t *testing.T) {
	t.Parallel()

	_, err := config.Decode(strings.NewReader("endpoints: []\n"))
	if err == nil || !strings.Contains(err.Error(), "at least one endpoint") {
		t.Fatalf("Decode() error = %v, want endpoint error", err)
	}
}
