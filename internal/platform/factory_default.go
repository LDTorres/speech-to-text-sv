//go:build !darwin && !linux

package platform

import (
	"github.com/LDTorres/speech-to-text-sv/internal/config"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
)

func newTriggerSource(_ config.ResolvedPlatform) trigger.Source {
	return trigger.NewStubSource()
}

func newRecorder(cfg config.AudioConfig, resolved config.ResolvedPlatform) audio.Recorder {
	return audio.NewFileRecorder(cfg.TempDir, cfg.FileName, cfg.SampleFormat)
}
