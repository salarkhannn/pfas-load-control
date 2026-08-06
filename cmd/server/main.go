package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/salarkhannn/pfas-load-control/internal/agent"
	"github.com/salarkhannn/pfas-load-control/internal/config"
	"github.com/salarkhannn/pfas-load-control/internal/database"
	"github.com/salarkhannn/pfas-load-control/internal/httpapi"
	"github.com/salarkhannn/pfas-load-control/internal/mireye"
	"github.com/salarkhannn/pfas-load-control/internal/observability"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration invalid", "error", err)
		os.Exit(1)
	}
	logger := slog.New(observability.NewRedactingHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))
	slog.SetDefault(logger)

	rootCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	startupCtx, cancelStartup := context.WithTimeout(rootCtx, 30*time.Second)
	defer cancelStartup()

	pool, err := database.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := database.Migrate(startupCtx, pool, logger); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	runtime, err := agent.NewRuntime(startupCtx, pool, mireye.NewClient(cfg.MireyeURL, cfg.MireyeToken, nil), logger)
	if err != nil {
		logger.Error("agent runtime initialization failed", "error", err)
		os.Exit(1)
	}
	if err := runtime.Jobs.Start(rootCtx); err != nil {
		logger.Error("agent runtime start failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           httpapi.NewRouter(runtime.Service, runtime.Lab, runtime.Policy, runtime.Fields, runtime.Evidence, runtime.Placement, runtime.Response, runtime.Packages, pool, logger, cfg.WebOrigin),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("API listening", "address", cfg.HTTPAddress)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown requested")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API stopped unexpectedly", "error", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("API shutdown failed", "error", err)
	}
	if err := runtime.Jobs.Stop(shutdownCtx); err != nil {
		logger.Error("agent runtime shutdown failed", "error", err)
	}
}
