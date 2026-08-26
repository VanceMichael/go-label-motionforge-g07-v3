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

	"github.com/VanceMichael/go-base-motionforge-g07/internal/annotation"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/clock"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/config"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/dataset"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/facility"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/httpapi"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/training"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rootCtx, stopSignal := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignal()
	database, err := storage.Open(rootCtx, storage.Options{Path: cfg.DatabasePath, MaxOpenConns: cfg.DatabaseMaxOpenConns, BusyTimeout: 5 * time.Second})
	if err != nil {
		return err
	}
	defer database.Close()
	realClock := clock.Real{}
	authService := auth.NewService(database, realClock, cfg.SessionTTL)
	api, err := httpapi.New(httpapi.Dependencies{
		Database:    database,
		Auth:        authService,
		Facilities:  facility.NewService(database, realClock, cfg.LeaseTTL),
		Captures:    capture.NewService(database, realClock, cfg.LeaseTTL),
		Streams:     stream.NewService(database, realClock),
		Annotations: annotation.NewService(database, realClock, cfg.LeaseTTL),
		Datasets:    dataset.NewService(database, realClock),
		Training:    training.NewService(database, realClock, cfg.LeaseTTL, cfg.WorkerRetryBase, cfg.WorkerMaxAttempts),
		Logger:      logger,
	})
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errorCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.Address, "database", cfg.DatabasePath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorCh <- err
			return
		}
		errorCh <- nil
	}()
	select {
	case err := <-errorCh:
		return err
	case <-rootCtx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return <-errorCh
}
