// Package monitor performs endpoint checks and stores in-memory results.
package monitor

import "time"

// State is the public endpoint health state.
type State string

const (
	StateUp       State = "UP"
	StateDegraded State = "DEGRADED"
	StateDown     State = "DOWN"
	StateUnknown  State = "UNKNOWN"
)

// Result records one scheduled endpoint check after retries.
type Result struct {
	State          State     `json:"state"`
	CheckedAt      time.Time `json:"checkedAt"`
	ResponseTimeMS float64   `json:"responseTimeMs"`
	StatusCode     int       `json:"statusCode,omitempty"`
	Error          string    `json:"error,omitempty"`
	Attempt        int       `json:"attempt"`
}

// StateChange represents a transition observed during the current process run.
type StateChange struct {
	From State     `json:"from"`
	To   State     `json:"to"`
	At   time.Time `json:"at"`
}

// EndpointStatus is a privacy-conscious API representation of one target.
type EndpointStatus struct {
	Name          string        `json:"name"`
	URL           string        `json:"url"`
	Method        string        `json:"method"`
	ExpectedCode  int           `json:"expectedStatus"`
	State         State         `json:"state"`
	ResponseTime  float64       `json:"responseTimeMs"`
	LastChecked   *time.Time    `json:"lastChecked"`
	StatusCode    int           `json:"statusCode,omitempty"`
	Error         string        `json:"error,omitempty"`
	UptimePercent float64       `json:"uptimePercent"`
	Checks        int           `json:"checks"`
	History       []Result      `json:"history,omitempty"`
	StateChanges  []StateChange `json:"stateChanges,omitempty"`
}

// Summary is the response body for GET /api/status.
type Summary struct {
	Service     string           `json:"service"`
	State       State            `json:"state"`
	GeneratedAt time.Time        `json:"generatedAt"`
	Endpoints   []EndpointStatus `json:"endpoints"`
}
