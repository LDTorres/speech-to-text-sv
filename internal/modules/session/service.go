package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/clipboard"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/notify"
	"github.com/LDTorres/speech-to-text-sv/internal/modules/transcribe"
	"go.uber.org/zap"
)

type State string

const (
	StateIdle       State = "idle"
	StateRecording  State = "recording"
	StateProcessing State = "processing"
)

var (
	ErrAlreadyRecording    = errors.New("session already recording")
	ErrNotRecording        = errors.New("session is not recording")
	ErrNoTranscript        = errors.New("no transcript available")
	ErrTranscriptionFailed = errors.New("session transcription failed")
	ErrPasteFailed         = errors.New("session paste failed")
	ErrBusy                = errors.New("session is processing")
	ErrInvalidState        = errors.New("session is in an invalid state")
)

type Status struct {
	State          State
	RetryAvailable bool
	LastTranscript string
}

type Service interface {
	StartRecording(ctx context.Context) error
	StopRecordingAndProcess(ctx context.Context) error
	ToggleRecording(ctx context.Context) error
	RetryLastPaste(ctx context.Context) error
	Status(ctx context.Context) Status
}

type state struct {
	current          State
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

	mu        sync.Mutex
	operation chan struct{}
	state     state
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
		operation:   make(chan struct{}, 1),
		state: state{
			current: StateIdle,
		},
	}
}

func (s *SessionService) StartRecording(ctx context.Context) error {
	if err := s.acquireOperation(ctx); err != nil {
		return err
	}
	defer s.releaseOperation()

	return s.startRecording(ctx)
}

func (s *SessionService) startRecording(ctx context.Context) error {
	s.logger.Info("recording start requested")

	if err := s.transitionToRecording(); err != nil {
		return err
	}

	if err := s.recorder.Start(ctx); err != nil {
		s.setState(StateIdle)
		return fmt.Errorf("start recording: %w", err)
	}

	s.notify(ctx, "recording started")
	s.logger.Info("recording started")

	return nil
}

func (s *SessionService) StopRecordingAndProcess(ctx context.Context) error {
	if err := s.acquireOperationBlocking(ctx); err != nil {
		return err
	}
	defer s.releaseOperation()

	return s.stopRecordingAndProcess(ctx)
}

func (s *SessionService) stopRecordingAndProcess(ctx context.Context) error {
	s.logger.Info("recording stop requested")

	if err := s.transitionToProcessing(); err != nil {
		return err
	}
	defer s.setState(StateIdle)

	recording, err := s.recorder.Stop(ctx)
	if err != nil {
		return fmt.Errorf("stop recording: %w", err)
	}
	defer func() {
		if recording.Path == "" {
			return
		}
		if err := os.Remove(recording.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("remove audio recording", zap.String("recording_path", recording.Path), zap.Error(err))
		}
	}()

	fields := s.recordingLogFields(recording)
	s.logger.Info("recording finalized", fields...)
	s.logger.Info("transcription started", fields...)

	transcript, err := s.transcriber.TranscribeFile(ctx, recording.Path)
	if err != nil {
		s.clearTranscript()
		s.logger.Error("transcription failed", append(fields, zap.Error(err))...)
		return fmt.Errorf("%w: %w", ErrTranscriptionFailed, err)
	}

	s.storeTranscript(transcript.Text)
	s.logger.Info(
		"transcription completed",
		append(fields,
			zap.Duration("transcription_duration", transcript.Duration),
			zap.Int("transcript_length", len(transcript.Text)),
		)...,
	)

	if err := s.clipboard.Copy(ctx, transcript.Text); err != nil {
		s.logger.Error("copy transcript failed", append(fields, zap.Error(err), zap.Int("transcript_length", len(transcript.Text)))...)
		return fmt.Errorf("copy transcript: %w", err)
	}

	if err := s.clipboard.Paste(ctx); err != nil {
		s.logger.Error("paste transcript failed", append(fields, zap.Error(err), zap.Int("transcript_length", len(transcript.Text)))...)
		return fmt.Errorf("%w: %w", ErrPasteFailed, err)
	}

	s.notify(ctx, "transcript pasted")
	s.logger.Info(
		"session completed",
		zap.Duration("transcription_duration", transcript.Duration),
		zap.Int("transcript_length", len(transcript.Text)),
	)

	return nil
}

func (s *SessionService) ToggleRecording(ctx context.Context) error {
	if err := s.acquireOperation(ctx); err != nil {
		return err
	}
	defer s.releaseOperation()

	switch s.Status(ctx).State {
	case StateIdle:
		return s.startRecording(ctx)
	case StateRecording:
		return s.stopRecordingAndProcess(ctx)
	case StateProcessing:
		return ErrBusy
	default:
		return ErrInvalidState
	}
}

func (s *SessionService) RetryLastPaste(ctx context.Context) error {
	if err := s.acquireOperation(ctx); err != nil {
		return err
	}
	defer s.releaseOperation()

	status := s.Status(ctx)
	if status.State == StateProcessing {
		return ErrBusy
	}
	if status.State != StateIdle {
		return ErrInvalidState
	}

	transcript, ok := s.lastTranscript()
	if !ok {
		return ErrNoTranscript
	}

	if err := s.clipboard.Copy(ctx, transcript); err != nil {
		return fmt.Errorf("copy transcript: %w", err)
	}

	if err := s.clipboard.Paste(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrPasteFailed, err)
	}

	s.notify(ctx, "transcript pasted")
	s.logger.Info("retry paste completed", zap.Int("transcript_length", len(transcript)))

	return nil
}

func (s *SessionService) acquireOperation(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case s.operation <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func (s *SessionService) acquireOperationBlocking(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case s.operation <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SessionService) releaseOperation() {
	<-s.operation
}

func (s *SessionService) notify(ctx context.Context, message string) {
	if err := s.notifier.Notify(ctx, message); err != nil {
		s.logger.Warn("send session notification", zap.String("message", message), zap.Error(err))
	}
}

func (s *SessionService) Status(ctx context.Context) Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Status{
		State:          s.state.current,
		RetryAvailable: s.state.retryEligible && s.state.lastTranscript != "",
		LastTranscript: s.state.lastTranscript,
	}
}

func (s *SessionService) HandleTriggerPressed(ctx context.Context) error {
	return s.StartRecording(ctx)
}

func (s *SessionService) HandleTriggerReleased(ctx context.Context) error {
	return s.StopRecordingAndProcess(ctx)
}

func (s *SessionService) transitionToRecording() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state.current {
	case StateIdle:
		s.transitionLocked(StateRecording)
		return nil
	case StateRecording:
		return ErrAlreadyRecording
	case StateProcessing:
		return ErrBusy
	default:
		return ErrInvalidState
	}
}

func (s *SessionService) transitionToProcessing() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state.current {
	case StateRecording:
		s.transitionLocked(StateProcessing)
		return nil
	case StateIdle:
		return ErrNotRecording
	case StateProcessing:
		return ErrBusy
	default:
		return ErrInvalidState
	}
}

func (s *SessionService) setState(next State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitionLocked(next)
}

func (s *SessionService) transitionLocked(next State) {
	if s.state.current == next {
		return
	}

	s.logger.Info(
		"session state changed",
		zap.String("from", string(s.state.current)),
		zap.String("to", string(next)),
	)
	s.state.current = next
}

func (s *SessionService) clearTranscript() {
	s.mu.Lock()
	s.state.lastTranscript = ""
	s.state.lastTranscriptAt = time.Time{}
	s.state.retryEligible = false
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

func (s *SessionService) recordingLogFields(recording audio.Recording) []zap.Field {
	fields := []zap.Field{
		zap.String("recording_path", recording.Path),
		zap.Time("recording_started_at", recording.StartedAt),
		zap.Time("recording_stopped_at", recording.StoppedAt),
		zap.Duration("recording_duration", recording.StoppedAt.Sub(recording.StartedAt)),
	}

	info, err := os.Stat(recording.Path)
	if err != nil {
		s.logger.Warn(
			"stat recording file",
			zap.String("recording_path", recording.Path),
			zap.Error(err),
		)
		return fields
	}

	if !info.IsDir() {
		fields = append(fields, zap.Int64("recording_size_bytes", info.Size()))
	}

	return fields
}
