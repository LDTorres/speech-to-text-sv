package darwin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"golang.design/x/hotkey"
)

type hotkeyBinding interface {
	Register() error
	Unregister() error
	Keydown() <-chan hotkey.Event
	Keyup() <-chan hotkey.Event
}

type hotkeyBindingFactory func(config.ResolvedHotkey) (hotkeyBinding, error)

type HotkeySource struct {
	hotkey  config.ResolvedHotkey
	factory hotkeyBindingFactory

	mu      sync.Mutex
	events  chan trigger.SourceEvent
	cancel  context.CancelFunc
	done    chan struct{}
	binding hotkeyBinding
}

func NewHotkeySource(hotkeyConfig config.ResolvedHotkey) *HotkeySource {
	return &HotkeySource{
		hotkey:  hotkeyConfig,
		factory: newHotkeyBinding,
		events:  make(chan trigger.SourceEvent, 8),
	}
}

func (s *HotkeySource) Events() <-chan trigger.SourceEvent {
	return s.events
}

func (s *HotkeySource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		return trigger.ErrSourceAlreadyStarted
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	binding, err := s.factory(s.hotkey)
	if err != nil {
		cancel()
		return fmt.Errorf("create mac hotkey binding: %w", err)
	}

	if err := binding.Register(); err != nil {
		cancel()
		return fmt.Errorf("register mac hotkey: %w", err)
	}

	s.cancel = cancel
	s.done = done
	s.binding = binding

	go s.run(runCtx, done, binding)

	return nil
}

func (s *HotkeySource) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	binding := s.binding
	if cancel == nil {
		s.mu.Unlock()
		return trigger.ErrSourceNotStarted
	}

	s.cancel = nil
	s.done = nil
	s.binding = nil
	s.mu.Unlock()

	cancel()

	if binding != nil {
		if err := binding.Unregister(); err != nil {
			return fmt.Errorf("unregister mac hotkey: %w", err)
		}
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *HotkeySource) run(ctx context.Context, done chan struct{}, binding hotkeyBinding) {
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		case <-binding.Keydown():
			if !emitSourceEvent(ctx, s.events, trigger.SourceEvent{
				Kind: trigger.SourceEventPress,
				At:   time.Now().UTC(),
			}) {
				return
			}
		case <-binding.Keyup():
			if !emitSourceEvent(ctx, s.events, trigger.SourceEvent{
				Kind: trigger.SourceEventRelease,
				At:   time.Now().UTC(),
			}) {
				return
			}
		}
	}
}

func emitSourceEvent(ctx context.Context, events chan trigger.SourceEvent, event trigger.SourceEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

type hotkeyAdapter struct {
	hk *hotkey.Hotkey
}

func newHotkeyBinding(hotkeyConfig config.ResolvedHotkey) (hotkeyBinding, error) {
	mods, err := parseHotkeyModifiers(hotkeyConfig.Modifiers)
	if err != nil {
		return nil, err
	}

	key, err := parseHotkeyKey(hotkeyConfig.Key)
	if err != nil {
		return nil, err
	}

	hk := hotkey.New(mods, key)
	return &hotkeyAdapter{hk: hk}, nil
}

func (a *hotkeyAdapter) Register() error {
	return a.hk.Register()
}

func (a *hotkeyAdapter) Unregister() error {
	return a.hk.Unregister()
}

func (a *hotkeyAdapter) Keydown() <-chan hotkey.Event {
	return a.hk.Keydown()
}

func (a *hotkeyAdapter) Keyup() <-chan hotkey.Event {
	return a.hk.Keyup()
}

func parseHotkeyModifiers(modifiers []string) ([]hotkey.Modifier, error) {
	parsed := make([]hotkey.Modifier, 0, len(modifiers))

	for _, modifier := range modifiers {
		switch strings.ToLower(strings.TrimSpace(modifier)) {
		case "cmd":
			parsed = append(parsed, hotkey.ModCmd)
		case "shift":
			parsed = append(parsed, hotkey.ModShift)
		case "ctrl":
			parsed = append(parsed, hotkey.ModCtrl)
		case "alt":
			parsed = append(parsed, hotkey.ModOption)
		default:
			return nil, fmt.Errorf("unsupported mac hotkey modifier %q", modifier)
		}
	}

	return parsed, nil
}

func parseHotkeyKey(key string) (hotkey.Key, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "space":
		return hotkey.KeySpace, nil
	case "tab":
		return hotkey.KeyTab, nil
	case "return":
		return hotkey.KeyReturn, nil
	case "escape":
		return hotkey.KeyEscape, nil
	case "a":
		return hotkey.KeyA, nil
	case "b":
		return hotkey.KeyB, nil
	case "c":
		return hotkey.KeyC, nil
	case "d":
		return hotkey.KeyD, nil
	case "e":
		return hotkey.KeyE, nil
	case "f":
		return hotkey.KeyF, nil
	case "g":
		return hotkey.KeyG, nil
	case "h":
		return hotkey.KeyH, nil
	case "i":
		return hotkey.KeyI, nil
	case "j":
		return hotkey.KeyJ, nil
	case "k":
		return hotkey.KeyK, nil
	case "l":
		return hotkey.KeyL, nil
	case "m":
		return hotkey.KeyM, nil
	case "n":
		return hotkey.KeyN, nil
	case "o":
		return hotkey.KeyO, nil
	case "p":
		return hotkey.KeyP, nil
	case "q":
		return hotkey.KeyQ, nil
	case "r":
		return hotkey.KeyR, nil
	case "s":
		return hotkey.KeyS, nil
	case "t":
		return hotkey.KeyT, nil
	case "u":
		return hotkey.KeyU, nil
	case "v":
		return hotkey.KeyV, nil
	case "w":
		return hotkey.KeyW, nil
	case "x":
		return hotkey.KeyX, nil
	case "y":
		return hotkey.KeyY, nil
	case "z":
		return hotkey.KeyZ, nil
	case "0":
		return hotkey.Key0, nil
	case "1":
		return hotkey.Key1, nil
	case "2":
		return hotkey.Key2, nil
	case "3":
		return hotkey.Key3, nil
	case "4":
		return hotkey.Key4, nil
	case "5":
		return hotkey.Key5, nil
	case "6":
		return hotkey.Key6, nil
	case "7":
		return hotkey.Key7, nil
	case "8":
		return hotkey.Key8, nil
	case "9":
		return hotkey.Key9, nil
	case "f1":
		return hotkey.KeyF1, nil
	case "f2":
		return hotkey.KeyF2, nil
	case "f3":
		return hotkey.KeyF3, nil
	case "f4":
		return hotkey.KeyF4, nil
	case "f5":
		return hotkey.KeyF5, nil
	case "f6":
		return hotkey.KeyF6, nil
	case "f7":
		return hotkey.KeyF7, nil
	case "f8":
		return hotkey.KeyF8, nil
	case "f9":
		return hotkey.KeyF9, nil
	case "f10":
		return hotkey.KeyF10, nil
	case "f11":
		return hotkey.KeyF11, nil
	case "f12":
		return hotkey.KeyF12, nil
	default:
		return 0, fmt.Errorf("unsupported mac hotkey key %q", key)
	}
}
