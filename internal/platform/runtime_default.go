//go:build !darwin

package platform

import "context"

func RunOnMain(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}
