package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LDTorres/speech-to-text-sv/internal/bootstrap"
	"go.uber.org/zap"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	boot, err := bootstrap.New(rootCtx)
	if err != nil {
		writeStartupError(err)
		os.Exit(1)
	}

	defer func() {
		if closeErr := boot.Close(); closeErr != nil {
			writeStartupError(closeErr)
		}
	}()

	if err := boot.Run(rootCtx); err != nil {
		writeStartupError(err)
		os.Exit(1)
	}
}

func writeStartupError(err error) {
	logger, buildErr := zap.NewProduction()
	if buildErr != nil {
		return
	}

	defer func() {
		_ = logger.Sync()
	}()

	logger.Error("application failed", zap.Error(err))
}
