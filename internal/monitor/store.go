package monitor

import (
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
)

const historyLimit = 50

type endpointRecord struct {
	endpoint config.Endpoint
	history  []Result
}

// Store is a concurrency-safe, bounded in-memory result repository.
type Store struct {
	mu      sync.RWMutex
	records map[string]*endpointRecord
	order   []string
}

// NewStore initializes UNKNOWN state for configured endpoints.
func NewStore(endpoints []config.Endpoint) *Store {
	records := make(map[string]*endpointRecord, len(endpoints))
	order := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		records[endpoint.Name] = &endpointRecord{endpoint: endpoint}
		order = append(order, endpoint.Name)
	}
	return &Store{records: records, order: order}
}

// Record appends one result, retains the latest 50, and returns the previous state.
func (store *Store) Record(name string, result Result) State {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[name]
	if !ok {
		return StateUnknown
	}
	previous := StateUnknown
	if len(record.history) > 0 {
		previous = record.history[len(record.history)-1].State
	}
	record.history = append(record.history, result)
	if len(record.history) > historyLimit {
		record.history = append([]Result(nil), record.history[len(record.history)-historyLimit:]...)
	}
	return previous
}

// All returns endpoint snapshots in configuration order.
func (store *Store) All(includeHistory bool) []EndpointStatus {
	store.mu.RLock()
	defer store.mu.RUnlock()
	statuses := make([]EndpointStatus, 0, len(store.order))
	for _, name := range store.order {
		statuses = append(statuses, buildStatus(store.records[name], includeHistory))
	}
	return statuses
}

// Get returns one endpoint snapshot by exact name.
func (store *Store) Get(name string, includeHistory bool) (EndpointStatus, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	record, ok := store.records[name]
	if !ok {
		return EndpointStatus{}, false
	}
	return buildStatus(record, includeHistory), true
}

func buildStatus(record *endpointRecord, includeHistory bool) EndpointStatus {
	status := EndpointStatus{
		Name:         record.endpoint.Name,
		URL:          publicURL(record.endpoint.URL),
		Method:       record.endpoint.Method,
		ExpectedCode: record.endpoint.ExpectedStatus,
		State:        StateUnknown,
		Checks:       len(record.history),
	}
	if len(record.history) == 0 {
		return status
	}

	current := record.history[len(record.history)-1]
	checkedAt := current.CheckedAt
	status.State = current.State
	status.ResponseTime = current.ResponseTimeMS
	status.LastChecked = &checkedAt
	status.StatusCode = current.StatusCode
	status.Error = current.Error
	status.UptimePercent = uptime(record.history)
	status.StateChanges = stateChanges(record.history)
	if includeHistory {
		status.History = append([]Result(nil), record.history...)
	}
	return status
}

func publicURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func uptime(history []Result) float64 {
	if len(history) == 0 {
		return 0
	}
	successful := 0
	for _, result := range history {
		if result.State == StateUp || result.State == StateDegraded {
			successful++
		}
	}
	return float64(successful) / float64(len(history)) * 100
}

func stateChanges(history []Result) []StateChange {
	if len(history) == 0 {
		return nil
	}
	changes := make([]StateChange, 0)
	previous := StateUnknown
	for _, result := range history {
		if result.State != previous {
			changes = append(changes, StateChange{From: previous, To: result.State, At: result.CheckedAt})
			previous = result.State
		}
	}
	if len(changes) > 10 {
		changes = changes[len(changes)-10:]
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].At.After(changes[j].At) })
	return changes
}

// OverallState calculates the most severe current state.
func OverallState(statuses []EndpointStatus) State {
	if len(statuses) == 0 {
		return StateUnknown
	}
	overall := StateUp
	for _, status := range statuses {
		switch status.State {
		case StateDown:
			return StateDown
		case StateUnknown:
			if overall != StateDegraded {
				overall = StateUnknown
			}
		case StateDegraded:
			overall = StateDegraded
		}
	}
	return overall
}

// Now exists to keep report generation consistent and testable at call boundaries.
func Now() time.Time {
	return time.Now().UTC()
}
