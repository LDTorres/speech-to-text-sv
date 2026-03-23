package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LDTorres/golang-api-template/internal/config"
	"github.com/LDTorres/golang-api-template/internal/logger"
	"github.com/LDTorres/golang-api-template/internal/modules/users"
	"github.com/LDTorres/golang-api-template/internal/server"
	"github.com/LDTorres/golang-api-template/internal/telemetry"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		writeStartupError(err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.App.Environment)
	if err != nil {
		writeStartupError(err)
		os.Exit(1)
	}

	defer func() {
		if syncErr := log.Sync(); syncErr != nil && !errors.Is(syncErr, syscall.ENOTTY) {
			log.Error("failed to sync logger", zap.Error(syncErr))
		}
	}()

	telemetryShutdown, err := telemetry.Setup(rootCtx, cfg.Telemetry)
	if err != nil {
		log.Fatal("failed to initialize telemetry", zap.Error(err))
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		if shutdownErr := telemetryShutdown(shutdownCtx); shutdownErr != nil {
			log.Error("failed to shutdown telemetry", zap.Error(shutdownErr))
		}
	}()

	db, err := openDatabase(rootCtx, cfg.Database.URL)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("failed to access sql database handle", zap.Error(err))
	}

	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.Error("failed to close database", zap.Error(closeErr))
		}
	}()

	repository := users.NewRepository(db)
	service := users.NewService(repository)
	handler := users.NewHandler(service)

	router := server.NewRouter(log, cfg.Server.RequestTimeout, handler)
	httpServer := server.NewHTTPServer(cfg.Server.Addr, router, server.Timeouts{
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	})

	serverErrors := make(chan error, 1)

	go func() {
		log.Info("starting http server", zap.String("addr", cfg.Server.Addr))
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case err = <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("http server failed", zap.Error(err))
		}
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatal("failed to shutdown http server", zap.Error(err))
	}

	log.Info("http server stopped")
}

func openDatabase(ctx context.Context, databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, err
	}

	return db, nil
}

func writeStartupError(err error) {
	log, buildErr := zap.NewProduction()
	if buildErr != nil {
		return
	}

	defer func() {
		_ = log.Sync()
	}()

	log.Error("application startup failed", zap.Error(err))
}
