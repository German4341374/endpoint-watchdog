package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/German4341374/endpoint-watchdog/internal/config"
	"github.com/German4341374/endpoint-watchdog/internal/monitor"
	"github.com/German4341374/endpoint-watchdog/internal/web"
)

var version = "0.1.0"

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("endpoint-watchdog %s\n", version)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(*configPath, logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(configPath string, logger *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	store := monitor.NewStore(cfg.Endpoints)
	checker := monitor.NewChecker(
		&http.Client{},
		cfg.Server.MaxConcurrent,
		cfg.Retry,
		cfg.Server.DegradedThreshold.Duration,
	)
	service := monitor.NewService(cfg.Endpoints, checker, store, logger)
	webServer, err := web.New(cfg.Server.Title, store, logger)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.Server.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.Address, err)
	}
	httpServer := &http.Server{
		Handler:           webServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	processContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()
	monitorContext, cancelMonitor := context.WithCancel(context.Background())
	defer cancelMonitor()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info(
			"HTTP server started",
			"address", listener.Addr().String(),
			"endpoints", len(cfg.Endpoints),
			"max_concurrent", cfg.Server.MaxConcurrent,
		)
		if serveErr := httpServer.Serve(listener); !errors.Is(serveErr, http.ErrServerClosed) {
			serverErrors <- serveErr
		}
	}()
	service.Start(monitorContext)

	select {
	case <-processContext.Done():
		logger.Info("shutdown signal received")
	case serveErr := <-serverErrors:
		cancelMonitor()
		service.Wait()
		return fmt.Errorf("serve HTTP: %w", serveErr)
	}

	cancelMonitor()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}
	service.Wait()
	logger.Info("graceful shutdown completed")
	return nil
}
