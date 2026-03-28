package linux

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
)

const linuxInputEventSize = 24

type eventReader interface {
	io.Reader
	io.Closer
}

type eventOpenFunc func(path string) (eventReader, error)

type EvdevSource struct {
	devicePath  string
	eventType   uint16
	eventCode   uint16
	activeValue int32
	open        eventOpenFunc

	mu     sync.Mutex
	events chan trigger.SourceEvent
	cancel context.CancelFunc
	done   chan struct{}
	reader eventReader
}

func NewEvdevSource(devicePath string, eventType uint16, eventCode uint16, activeValue int32) *EvdevSource {
	return &EvdevSource{
		devicePath:  devicePath,
		eventType:   eventType,
		eventCode:   eventCode,
		activeValue: activeValue,
		open: func(path string) (eventReader, error) {
			return os.Open(path)
		},
		events: make(chan trigger.SourceEvent, 8),
	}
}

func (s *EvdevSource) Events() <-chan trigger.SourceEvent {
	return s.events
}

func (s *EvdevSource) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		return trigger.ErrSourceAlreadyStarted
	}

	reader, err := s.open(s.devicePath)
	if err != nil {
		return fmt.Errorf("open evdev device: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	s.cancel = cancel
	s.done = done
	s.reader = reader

	go s.run(runCtx, done, reader)

	return nil
}

func (s *EvdevSource) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	reader := s.reader
	if cancel == nil {
		s.mu.Unlock()
		return trigger.ErrSourceNotStarted
	}

	s.cancel = nil
	s.done = nil
	s.reader = nil
	s.mu.Unlock()

	cancel()
	if reader != nil {
		_ = reader.Close()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *EvdevSource) run(ctx context.Context, done chan struct{}, reader eventReader) {
	defer close(done)

	buffer := make([]byte, linuxInputEventSize)
	for {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			if shouldIgnoreReadError(ctx, err) {
				return
			}
			return
		}

		rawEvent := decodeInputEvent(buffer)
		sourceEvent, ok := s.mapEvent(rawEvent)
		if !ok {
			continue
		}

		select {
		case s.events <- sourceEvent:
		case <-ctx.Done():
			return
		}
	}
}

func (s *EvdevSource) mapEvent(rawEvent InputEvent) (trigger.SourceEvent, bool) {
	if rawEvent.Type != s.eventType || rawEvent.Code != s.eventCode {
		return trigger.SourceEvent{}, false
	}

	switch rawEvent.Value {
	case s.activeValue:
		return trigger.SourceEvent{
			Kind: trigger.SourceEventPress,
			At:   rawEvent.At,
		}, true
	case 0:
		return trigger.SourceEvent{
			Kind: trigger.SourceEventRelease,
			At:   rawEvent.At,
		}, true
	default:
		return trigger.SourceEvent{}, false
	}
}

type InputEvent struct {
	At    time.Time
	Type  uint16
	Code  uint16
	Value int32
}

func decodeInputEvent(buffer []byte) InputEvent {
	seconds := int64(binary.LittleEndian.Uint64(buffer[0:8]))
	micros := int64(binary.LittleEndian.Uint64(buffer[8:16]))

	return InputEvent{
		At:    time.Unix(seconds, micros*int64(time.Microsecond)).UTC(),
		Type:  binary.LittleEndian.Uint16(buffer[16:18]),
		Code:  binary.LittleEndian.Uint16(buffer[18:20]),
		Value: int32(binary.LittleEndian.Uint32(buffer[20:24])),
	}
}

func shouldIgnoreReadError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	if ctx.Err() != nil {
		return true
	}

	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrClosed)
}
