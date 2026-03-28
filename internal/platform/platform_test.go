//go:build linux

package platform

import (
	"testing"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	linuxplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/linux"
	"github.com/stretchr/testify/require"
)

func TestPlatform_NewTriggerWatcher_UsesLinuxHotkey(t *testing.T) {
	source := newTriggerSource(config.ResolvedPlatform{
		Profile: config.PlatformProfileLinux,
		Trigger: config.ResolvedTrigger{
			Mode: config.TriggerModeHold,
			Hotkey: config.ResolvedHotkey{
				Modifiers: []string{"ctrl", "shift"},
				Key:       "space",
			},
		},
	})

	_, ok := source.(*linuxplatform.HotkeySource)
	require.True(t, ok)
}

func TestPlatform_NewTriggerWatcher_UsesSteamDeckHotkey(t *testing.T) {
	source := newTriggerSource(config.ResolvedPlatform{
		Profile: config.PlatformProfileSteamDeck,
		Trigger: config.ResolvedTrigger{
			Mode: config.TriggerModeToggle,
			Hotkey: config.ResolvedHotkey{
				Modifiers: []string{},
				Key:       "f12",
			},
		},
	})

	_, ok := source.(*linuxplatform.HotkeySource)
	require.True(t, ok)
}

func TestPlatform_NewRecorder_UsesPWRecord(t *testing.T) {
	recorder := newRecorder(config.AudioConfig{
		TempDir:  t.TempDir(),
		FileName: "recording.wav",
	}, config.ResolvedPlatform{
		Profile: config.PlatformProfileLinux,
		Audio: config.ResolvedAudio{
			Backend: config.AudioBackendPWRecord,
		},
	})

	_, ok := recorder.(*linuxplatform.PWRecordRecorder)
	require.True(t, ok)
}
