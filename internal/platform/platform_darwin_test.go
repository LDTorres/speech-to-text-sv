//go:build darwin

package platform

import (
	"testing"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	darwinplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/darwin"
	"github.com/stretchr/testify/require"
)

func TestPlatform_NewTriggerWatcher_UsesMacHotkey(t *testing.T) {
	source := newTriggerSource(config.ResolvedPlatform{
		Profile: config.PlatformProfileMacOSDev,
		Trigger: config.ResolvedTrigger{
			Source: config.TriggerSourceHotkey,
			Hotkey: config.ResolvedHotkey{
				Modifiers: []string{"cmd", "shift"},
				Key:       "space",
			},
		},
	})

	_, ok := source.(*darwinplatform.HotkeySource)
	require.True(t, ok)
}

func TestPlatform_NewRecorder_UsesMacCapture(t *testing.T) {
	recorder := newRecorder(config.AudioConfig{
		TempDir:      t.TempDir(),
		FileName:     "recording.wav",
		SampleFormat: "wav",
	}, config.ResolvedPlatform{
		Profile: config.PlatformProfileMacOSDev,
		Audio: config.ResolvedAudio{
			Backend: config.AudioBackendMacOSCapture,
		},
	})

	_, ok := recorder.(*darwinplatform.Recorder)
	require.True(t, ok)
}
