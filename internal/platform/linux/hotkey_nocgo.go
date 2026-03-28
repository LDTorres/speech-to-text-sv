//go:build linux && (!cgo || !x11hotkey)

package linux

import (
	"context"
	"errors"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
)

var ErrHotkeyUnavailable = errors.New("linux hotkey source requires a build with cgo and x11hotkey support")

type HotkeySource struct {
	events chan trigger.SourceEvent
}

func NewHotkeySource(hotkeyConfig config.ResolvedHotkey) *HotkeySource {
	return &HotkeySource{
		events: make(chan trigger.SourceEvent, 1),
	}
}

func (s *HotkeySource) Events() <-chan trigger.SourceEvent {
	return s.events
}

func (s *HotkeySource) Start(ctx context.Context) error {
	return ErrHotkeyUnavailable
}

func (s *HotkeySource) Stop(ctx context.Context) error {
	return trigger.ErrSourceNotStarted
}
