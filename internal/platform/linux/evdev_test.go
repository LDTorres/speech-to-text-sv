package linux

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/trigger"
	"github.com/stretchr/testify/require"
)

func TestEvdevSource_EmitsPressAndRelease(t *testing.T) {
	t.Parallel()

	device := newFakeEventDevice()
	source := NewEvdevSource("/dev/input/event0", 1, 1337, 1)
	source.open = func(path string) (eventReader, error) {
		return device, nil
	}

	require.NoError(t, source.Start(context.Background()))
	defer func() {
		require.NoError(t, source.Stop(context.Background()))
	}()

	device.send(encodeInputEvent(1, 1337, 1))
	press := readLinuxSourceEvent(t, source.Events())
	require.Equal(t, trigger.SourceEventPress, press.Kind)

	device.send(encodeInputEvent(1, 1337, 0))
	release := readLinuxSourceEvent(t, source.Events())
	require.Equal(t, trigger.SourceEventRelease, release.Kind)
}

func TestEvdevSource_StopUnblocksRunLoop(t *testing.T) {
	t.Parallel()

	device := newFakeEventDevice()
	source := NewEvdevSource("/dev/input/event0", 1, 1337, 1)
	source.open = func(path string) (eventReader, error) {
		return device, nil
	}

	require.NoError(t, source.Start(context.Background()))
	require.NoError(t, source.Stop(context.Background()))
}

func TestDecodeInputEvent(t *testing.T) {
	t.Parallel()

	event := decodeInputEvent(encodeInputEvent(1, 1337, 1))

	require.Equal(t, uint16(1), event.Type)
	require.Equal(t, uint16(1337), event.Code)
	require.Equal(t, int32(1), event.Value)
}

func TestShouldIgnoreReadError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.True(t, shouldIgnoreReadError(ctx, errors.New("boom")))
}

type fakeEventDevice struct {
	reads  chan []byte
	closed chan struct{}
}

func newFakeEventDevice() *fakeEventDevice {
	return &fakeEventDevice{
		reads:  make(chan []byte, 4),
		closed: make(chan struct{}),
	}
}

func (d *fakeEventDevice) Read(p []byte) (int, error) {
	select {
	case data := <-d.reads:
		return copy(p, data), nil
	case <-d.closed:
		return 0, io.EOF
	}
}

func (d *fakeEventDevice) Close() error {
	select {
	case <-d.closed:
		return nil
	default:
		close(d.closed)
		return nil
	}
}

func (d *fakeEventDevice) send(data []byte) {
	d.reads <- data
}

func encodeInputEvent(eventType uint16, eventCode uint16, value int32) []byte {
	buffer := make([]byte, linuxInputEventSize)
	now := time.Now().UTC()
	binary.LittleEndian.PutUint64(buffer[0:8], uint64(now.Unix()))
	binary.LittleEndian.PutUint64(buffer[8:16], uint64(now.Nanosecond()/1000))
	binary.LittleEndian.PutUint16(buffer[16:18], eventType)
	binary.LittleEndian.PutUint16(buffer[18:20], eventCode)
	binary.LittleEndian.PutUint32(buffer[20:24], uint32(value))
	return buffer
}

func readLinuxSourceEvent(t *testing.T, events <-chan trigger.SourceEvent) trigger.SourceEvent {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("expected source event")
		return trigger.SourceEvent{}
	}
}
