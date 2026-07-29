package monitor_test

import (
	"testing"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
	"github.com/German4341374/endpoint-watchdog/internal/monitor"
)

func storeEndpoint() config.Endpoint {
	return config.Endpoint{
		Name:           "API",
		URL:            "https://api.example.test/health?token=not-exposed",
		Method:         "GET",
		ExpectedStatus: 200,
	}
}

func TestStoreStartsWithUnknownState(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore([]config.Endpoint{storeEndpoint()})

	status, found := store.Get("API", true)

	if !found || status.State != monitor.StateUnknown || status.Checks != 0 {
		t.Fatalf("Get() = %+v, %v, want UNKNOWN", status, found)
	}
}

func TestStoreCalculatesCurrentRunUptime(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore([]config.Endpoint{storeEndpoint()})
	now := time.Now().UTC()
	store.Record("API", monitor.Result{State: monitor.StateUp, CheckedAt: now})
	store.Record("API", monitor.Result{State: monitor.StateDegraded, CheckedAt: now.Add(time.Second)})
	store.Record("API", monitor.Result{State: monitor.StateDown, CheckedAt: now.Add(2 * time.Second)})

	status, _ := store.Get("API", true)

	if status.UptimePercent < 66.6 || status.UptimePercent > 66.7 {
		t.Fatalf("UptimePercent = %f, want about 66.67", status.UptimePercent)
	}
}

func TestStoreRetainsOnlyLatestFiftyResults(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore([]config.Endpoint{storeEndpoint()})
	now := time.Now().UTC()
	for index := range 60 {
		store.Record("API", monitor.Result{
			State:     monitor.StateUp,
			CheckedAt: now.Add(time.Duration(index) * time.Second),
			Attempt:   index + 1,
		})
	}

	status, _ := store.Get("API", true)

	if len(status.History) != 50 || status.History[0].Attempt != 11 {
		t.Fatalf("history length/first = %d/%d, want 50/11", len(status.History), status.History[0].Attempt)
	}
}

func TestStoreTracksRecentStateChanges(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore([]config.Endpoint{storeEndpoint()})
	now := time.Now().UTC()
	store.Record("API", monitor.Result{State: monitor.StateUp, CheckedAt: now})
	store.Record("API", monitor.Result{State: monitor.StateUp, CheckedAt: now.Add(time.Second)})
	store.Record("API", monitor.Result{State: monitor.StateDown, CheckedAt: now.Add(2 * time.Second)})

	status, _ := store.Get("API", false)

	if len(status.StateChanges) != 2 {
		t.Fatalf("state changes = %d, want 2", len(status.StateChanges))
	}
	if status.StateChanges[0].From != monitor.StateUp || status.StateChanges[0].To != monitor.StateDown {
		t.Fatalf("latest state change = %+v", status.StateChanges[0])
	}
}

func TestStoreDoesNotExposeURLQuery(t *testing.T) {
	t.Parallel()
	store := monitor.NewStore([]config.Endpoint{storeEndpoint()})

	status, _ := store.Get("API", false)

	if status.URL != "https://api.example.test/health" {
		t.Fatalf("URL = %q, want query removed", status.URL)
	}
}

func TestOverallStateUsesMostSevereState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		statuses []monitor.EndpointStatus
		want     monitor.State
	}{
		{"empty", nil, monitor.StateUnknown},
		{"all up", []monitor.EndpointStatus{{State: monitor.StateUp}}, monitor.StateUp},
		{"degraded", []monitor.EndpointStatus{{State: monitor.StateUp}, {State: monitor.StateDegraded}}, monitor.StateDegraded},
		{"unknown", []monitor.EndpointStatus{{State: monitor.StateUp}, {State: monitor.StateUnknown}}, monitor.StateUnknown},
		{"down", []monitor.EndpointStatus{{State: monitor.StateDegraded}, {State: monitor.StateDown}}, monitor.StateDown},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := monitor.OverallState(testCase.statuses); got != testCase.want {
				t.Fatalf("OverallState() = %s, want %s", got, testCase.want)
			}
		})
	}
}
