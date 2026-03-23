package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/clipboard"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"go.uber.org/zap"
)

var (
	ErrAlreadyRecording    = errors.New("session already recording")
	ErrNotRecording        = errors.New("session is not recording")
	ErrNoTranscript        = errors.New("no transcript available")
	ErrTranscriptionFailed = errors.New("session transcription failed")
	ErrPasteFailed         = errors.New("session paste failed")
)

type Service interface {
	HandleTriggerPressed(ctx context.Context) error
	HandleTriggerReleased(ctx context.Context) error
	RetryLastPaste(ctx context.Context) error
}

type state struct {
	recordingActive  bool
	lastTranscript   string
	lastTranscriptAt time.Time
	retryEligible    bool
}

type SessionService struct {
	logger      *zap.Logger
	recorder    audio.Recorder
	transcriber transcribe.Transcriber
	clipboard   clipboard.Clipboard
	notifier    notify.Notifier

	mu    sync.Mutex
	state state
}

func NewService(
	logger *zap.Logger,
	recorder audio.Recorder,
	transcriber transcribe.Transcriber,
	clipboard clipboard.Clipboard,
	notifier notify.Notifier,
) *SessionService {
	return &SessionService{
		logger:      logger,
		recorder:    recorder,
		transcriber: transcriber,
		clipboard:   clipboard,
		notifier:    notifier,
	}
}

func (s *SessionService) HandleTriggerPressed(ctx context.Context) error {
	if err := s.reserveRecording(); err != nil {
		return err
	}

	if err := s.recorder.Start(ctx); err != nil {
		s.releaseRecording()
		return fmt.Errorf("start recording: %w", err)
	}

	_ = s.notifier.Notify(ctx, "recording started")
	s.logger.Info("recording started")

	return nil
}

func (s *SessionService) HandleTriggerReleased(ctx context.Context) error {
	if err := s.finishRecording(); err != nil {
		return err
	}

	recording, err := s.recorder.Stop(ctx)
	if err != nil {
		return fmt.Errorf("stop recording: %w", err)
	}

	transcript, err := s.transcriber.TranscribeFile(ctx, recording.Path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrTranscriptionFailed, err)
	}

	s.storeTranscript(transcript.Text)

	if err := s.clipboard.Copy(ctx, transcript.Text); err != nil {
		return fmt.Errorf("copy transcript: %w", err)
	}

	if err := s.clipboard.Paste(ctx); err != nil {
		return fmt.Errorf("%w: %s", ErrPasteFailed, err)
	}

	_ = s.notifier.Notify(ctx, "transcript pasted")
	s.logger.Info(
		"session completed",
		zap.Duration("transcription_duration", transcript.Duration),
		zap.Int("transcript_length", len(transcript.Text)),
	)

	return nil
}

func (s *SessionService) RetryLastPaste(ctx context.Context) error {
	transcript, ok := s.lastTranscript()
	if !ok {
		return ErrNoTranscript
	}

	if err := s.clipboard.Copy(ctx, transcript); err != nil {
		return fmt.Errorf("copy transcript: %w", err)
	}

	if err := s.clipboard.Paste(ctx); err != nil {
		return fmt.Errorf("%w: %s", ErrPasteFailed, err)
	}

	_ = s.notifier.Notify(ctx, "transcript pasted")
	s.logger.Info("retry paste completed", zap.Int("transcript_length", len(transcript)))

	return nil
}

func (s *SessionService) reserveRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.recordingActive {
		return ErrAlreadyRecording
	}

	s.state.recordingActive = true

	return nil
}

func (s *SessionService) finishRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.state.recordingActive {
		return ErrNotRecording
	}

	s.state.recordingActive = false

	return nil
}

func (s *SessionService) releaseRecording() {
	s.mu.Lock()
	s.state.recordingActive = false
	s.mu.Unlock()
}

func (s *SessionService) storeTranscript(text string) {
	s.mu.Lock()
	s.state.lastTranscript = text
	s.state.lastTranscriptAt = time.Now().UTC()
	s.state.retryEligible = true
	s.mu.Unlock()
}

func (s *SessionService) lastTranscript() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.state.retryEligible || s.state.lastTranscript == "" {
		return "", false
	}

	return s.state.lastTranscript, true
}
