package clipboard

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"
)

var (
	ErrNothingCopied = errors.New("clipboard does not contain any text")
	ErrPasteDisabled = errors.New("paste injection disabled")
)

type Clipboard interface {
	Copy(ctx context.Context, text string) error
	Paste(ctx context.Context) error
}

type StubClipboard struct {
	logger      *zap.Logger
	enablePaste bool

	mu       sync.Mutex
	lastText string
}

func NewStub(logger *zap.Logger, enablePaste bool) *StubClipboard {
	return &StubClipboard{
		logger:      logger,
		enablePaste: enablePaste,
	}
}

func (c *StubClipboard) Copy(ctx context.Context, text string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	c.lastText = text
	c.mu.Unlock()

	c.logger.Info("clipboard copy attempted", zap.Int("text_length", len(text)))

	return nil
}

func (c *StubClipboard) Paste(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	text := c.lastText
	c.mu.Unlock()

	if text == "" {
		return ErrNothingCopied
	}

	if !c.enablePaste {
		return ErrPasteDisabled
	}

	c.logger.Info("clipboard paste attempted", zap.Int("text_length", len(text)))

	return nil
}
