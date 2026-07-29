package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
)

// Service schedules independent endpoint checks.
type Service struct {
	endpoints []config.Endpoint
	checker   *Checker
	store     *Store
	logger    *slog.Logger
	waitGroup sync.WaitGroup
}

// NewService constructs a stateless scheduler around a checker and store.
func NewService(
	endpoints []config.Endpoint,
	checker *Checker,
	store *Store,
	logger *slog.Logger,
) *Service {
	return &Service{endpoints: endpoints, checker: checker, store: store, logger: logger}
}

// Start launches one independent schedule per endpoint and returns immediately.
func (service *Service) Start(ctx context.Context) {
	for _, endpoint := range service.endpoints {
		service.waitGroup.Add(1)
		go service.runEndpoint(ctx, endpoint)
	}
}

// Wait blocks until every scheduler exits after context cancellation.
func (service *Service) Wait() {
	service.waitGroup.Wait()
}

func (service *Service) runEndpoint(ctx context.Context, endpoint config.Endpoint) {
	defer service.waitGroup.Done()
	service.execute(ctx, endpoint)

	ticker := time.NewTicker(endpoint.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.execute(ctx, endpoint)
		}
	}
}

func (service *Service) execute(ctx context.Context, endpoint config.Endpoint) {
	result := service.checker.Check(ctx, endpoint)
	if ctx.Err() != nil {
		return
	}
	previous := service.store.Record(endpoint.Name, result)
	attributes := []any{
		"endpoint", endpoint.Name,
		"state", result.State,
		"status_code", result.StatusCode,
		"response_time_ms", result.ResponseTimeMS,
		"attempt", result.Attempt,
	}
	if result.Error != "" {
		attributes = append(attributes, "error", result.Error)
	}
	if result.State == StateDown {
		service.logger.Warn("endpoint check completed", attributes...)
	} else {
		service.logger.Info("endpoint check completed", attributes...)
	}
	if previous != result.State {
		service.logger.Info(
			"endpoint state changed",
			"endpoint", endpoint.Name,
			"from", previous,
			"to", result.State,
		)
	}
}
