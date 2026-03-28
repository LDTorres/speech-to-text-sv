//go:build linux

package platform

import (
	"testing"

	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	linuxplatform "github.com/LDTorres/speech-to-text-sv/internal/platform/linux"
	"github.com/stretchr/testify/require"
)

func TestPlatform_NewTriggerWatcher_UsesSteamDeckEvdev(t *testing.T) {
	source := newTriggerSource(config.ResolvedPlatform{
		Profile: config.PlatformProfileSteamDeck,
		Trigger: config.ResolvedTrigger{
			Source:      config.TriggerSourceSteam,
			DevicePath:  "/dev/input/event0",
			EventType:   1,
			EventCode:   1337,
			ActiveValue: 1,
		},
	})

	_, ok := source.(*linuxplatform.EvdevSource)
	require.True(t, ok)
}

func TestPlatform_NewRecorder_UsesPWRecord(t *testing.T) {
	recorder := newRecorder(config.AudioConfig{
		TempDir:  t.TempDir(),
		FileName: "recording.wav",
	}, config.ResolvedPlatform{
		Profile: config.PlatformProfileSteamDeck,
		Audio: config.ResolvedAudio{
			Backend: config.AudioBackendPWRecord,
		},
	})

	_, ok := recorder.(*linuxplatform.PWRecordRecorder)
	require.True(t, ok)
}

func TestPlatform_NewTriggerWatcher_UsesStubForLinuxDesktopDefault(t *testing.T) {
	source := newTriggerSource(config.ResolvedPlatform{
		Profile: config.PlatformProfileLinuxDesktop,
		Trigger: config.ResolvedTrigger{
			Source: config.TriggerSourceStub,
		},
	})

	_, ok := source.(*trigger.StubSource)
	require.True(t, ok)
}
