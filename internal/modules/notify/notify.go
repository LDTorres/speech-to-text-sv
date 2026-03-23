package notify

import "context"

type Notifier interface {
	Notify(ctx context.Context, message string) error
}

type NoopNotifier struct{}

func NewNoop() *NoopNotifier {
	return &NoopNotifier{}
}

func (n *NoopNotifier) Notify(ctx context.Context, message string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
