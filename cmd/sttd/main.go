package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LDTorres/speech-to-text-sv/internal/bootstrap"
	"github.com/LDTorres/speech-to-text-sv/internal/platform"
	"go.uber.org/zap"
)

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := platform.RunOnMain(rootCtx, run); err != nil {
		writeStartupError(err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	boot, err := bootstrap.New(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := boot.Close(); closeErr != nil {
			writeStartupError(closeErr)
		}
	}()

	return boot.Run(ctx)
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
