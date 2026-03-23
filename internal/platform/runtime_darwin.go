//go:build darwin

package platform

import (
	"context"

	"golang.design/x/hotkey/mainthread"
)

func RunOnMain(ctx context.Context, run func(context.Context) error) error {
	var runErr error

	mainthread.Init(func() {
		runErr = run(ctx)
	})

	return runErr
}
