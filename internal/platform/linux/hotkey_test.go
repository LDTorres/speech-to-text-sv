//go:build linux && cgo && x11hotkey

package linux

import (
	"context"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"github.com/stretchr/testify/require"
	"golang.design/x/hotkey"
)

func TestLinuxHotkeySource_KeydownToPressAndKeyupToRelease(t *testing.T) {
	t.Parallel()

	binding := newFakeBinding()
	source := NewHotkeySource(config.ResolvedHotkey{
		Modifiers: []string{"ctrl"},
		Key:       "f13",
	})
	source.factory = func(config.ResolvedHotkey) (hotkeyBinding, error) {
		return binding, nil
	}

	require.NoError(t, source.Start(context.Background()))
	defer func() {
		require.NoError(t, source.Stop(context.Background()))
	}()

	binding.keydown <- hotkey.Event{}
	press := readHotkeySourceEvent(t, source.Events())
	require.Equal(t, trigger.SourceEventPress, press.Kind)

	binding.keyup <- hotkey.Event{}
	release := readHotkeySourceEvent(t, source.Events())
	require.Equal(t, trigger.SourceEventRelease, release.Kind)
}

func TestLinuxHotkeySource_StopUnregistersCleanly(t *testing.T) {
	t.Parallel()

	binding := newFakeBinding()
	source := NewHotkeySource(config.ResolvedHotkey{
		Modifiers: []string{"ctrl"},
		Key:       "f13",
	})
	source.factory = func(config.ResolvedHotkey) (hotkeyBinding, error) {
		return binding, nil
	}

	require.NoError(t, source.Start(context.Background()))
	require.NoError(t, source.Stop(context.Background()))
	require.Equal(t, 1, binding.unregisterCalls)
}

func readHotkeySourceEvent(t *testing.T, events <-chan trigger.SourceEvent) trigger.SourceEvent {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("expected source event")
		return trigger.SourceEvent{}
	}
}

type fakeBinding struct {
	keydown         chan hotkey.Event
	keyup           chan hotkey.Event
	unregisterCalls int
}

func newFakeBinding() *fakeBinding {
	return &fakeBinding{
		keydown: make(chan hotkey.Event, 1),
		keyup:   make(chan hotkey.Event, 1),
	}
}

func (f *fakeBinding) Register() error { return nil }

func (f *fakeBinding) Unregister() error {
	f.unregisterCalls++
	return nil
}

func (f *fakeBinding) Keydown() <-chan hotkey.Event { return f.keydown }

func (f *fakeBinding) Keyup() <-chan hotkey.Event { return f.keyup }
