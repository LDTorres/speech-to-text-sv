package darwin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/audio"
	"github.com/gen2brain/malgo"
)

type macCaptureFactory interface {
	Open(inputDevice string, onData func([]byte)) (macCaptureSession, error)
}

type macCaptureSession interface {
	Start() error
	Stop() error
	Close() error
	SampleRate() uint32
	Channels() uint32
}

type Recorder struct {
	tempDir      string
	fileName     string
	sampleFormat string
	inputDevice  string
	factory      macCaptureFactory

	mu        sync.Mutex
	recording bool
	startedAt time.Time
	buffer    []byte
	session   macCaptureSession
}

func NewRecorder(tempDir string, fileName string, sampleFormat string, inputDevice string) *Recorder {
	return &Recorder{
		tempDir:      tempDir,
		fileName:     fileName,
		sampleFormat: sampleFormat,
		inputDevice:  inputDevice,
		factory:      malgoCaptureFactory{},
	}
}

func (r *Recorder) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	if r.recording {
		r.mu.Unlock()
		return audio.ErrAlreadyRecording
	}
	r.buffer = nil
	r.mu.Unlock()

	session, err := r.factory.Open(r.inputDevice, func(data []byte) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.buffer = append(r.buffer, data...)
	})
	if err != nil {
		return err
	}

	if err := session.Start(); err != nil {
		_ = session.Close()
		return err
	}

	r.mu.Lock()
	r.recording = true
	r.startedAt = time.Now().UTC()
	r.session = session
	r.mu.Unlock()

	return nil
}

func (r *Recorder) Stop(ctx context.Context) (audio.Recording, error) {
	select {
	case <-ctx.Done():
		return audio.Recording{}, ctx.Err()
	default:
	}

	r.mu.Lock()
	if !r.recording || r.session == nil {
		r.mu.Unlock()
		return audio.Recording{}, audio.ErrNotRecording
	}

	session := r.session
	recording := audio.Recording{
		Path:      filepath.Join(r.tempDir, r.fileName),
		StartedAt: r.startedAt,
		StoppedAt: time.Now().UTC(),
	}
	r.recording = false
	r.startedAt = time.Time{}
	r.session = nil
	r.mu.Unlock()

	if err := session.Stop(); err != nil {
		_ = session.Close()
		return audio.Recording{}, fmt.Errorf("stop mac audio capture: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	r.mu.Lock()
	pcmData := append([]byte(nil), r.buffer...)
	r.buffer = nil
	r.mu.Unlock()

	if err := writePCMAsWAV(recording.Path, pcmData, session.SampleRate(), session.Channels()); err != nil {
		return audio.Recording{}, err
	}

	return recording, nil
}

type malgoCaptureFactory struct{}

func (f malgoCaptureFactory) Open(inputDevice string, onData func([]byte)) (macCaptureSession, error) {
	session, err := newMalgoCaptureSession(inputDevice, onData)
	if err != nil {
		return nil, err
	}

	return session, nil
}

type malgoSession struct {
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	sampleRate uint32
	channels   uint32
}

func newMalgoCaptureSession(inputDevice string, onData func([]byte)) (*malgoSession, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init mac audio context: %w", err)
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = 16000

	var selectedID malgo.DeviceID
	if inputDevice != "" {
		devices, err := ctx.Context.Devices(malgo.Capture)
		if err != nil {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("list mac audio devices: %w", err)
		}

		found := false
		for _, device := range devices {
			if device.Name() == inputDevice {
				selectedID = device.ID
				found = true
				break
			}
		}

		if !found {
			_ = ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("mac audio input device %q not found", inputDevice)
		}

		deviceConfig.Capture.DeviceID = unsafe.Pointer(&selectedID)
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: func(outputSamples []byte, inputSamples []byte, framecount uint32) {
			if len(inputSamples) == 0 {
				return
			}

			copied := append([]byte(nil), inputSamples...)
			onData(copied)
		},
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("init mac audio device: %w", err)
	}

	return &malgoSession{
		ctx:        ctx,
		device:     device,
		sampleRate: deviceConfig.SampleRate,
		channels:   deviceConfig.Capture.Channels,
	}, nil
}

func (s *malgoSession) Start() error {
	return s.device.Start()
}

func (s *malgoSession) Stop() error {
	return s.device.Stop()
}

func (s *malgoSession) Close() error {
	if s.device != nil {
		s.device.Uninit()
	}
	if s.ctx != nil {
		if err := s.ctx.Uninit(); err != nil {
			s.ctx.Free()
			return err
		}
		s.ctx.Free()
	}
	return nil
}

func (s *malgoSession) SampleRate() uint32 {
	return s.sampleRate
}

func (s *malgoSession) Channels() uint32 {
	return s.channels
}

func writePCMAsWAV(path string, pcmData []byte, sampleRate uint32, channels uint32) error {
	return writeWAVFile(path, pcmData, sampleRate, channels, 16)
}

func writeWAVFile(path string, pcmData []byte, sampleRate uint32, channels uint32, bitsPerSample uint16) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create audio temp dir: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create wav file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	blockAlign := uint16(channels) * bitsPerSample / 8
	dataSize := uint32(len(pcmData))
	chunkSize := 36 + dataSize

	header := []byte{
		'R', 'I', 'F', 'F',
		byte(chunkSize), byte(chunkSize >> 8), byte(chunkSize >> 16), byte(chunkSize >> 24),
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		16, 0, 0, 0,
		1, 0,
		byte(channels), byte(channels >> 8),
		byte(sampleRate), byte(sampleRate >> 8), byte(sampleRate >> 16), byte(sampleRate >> 24),
		byte(byteRate), byte(byteRate >> 8), byte(byteRate >> 16), byte(byteRate >> 24),
		byte(blockAlign), byte(blockAlign >> 8),
		byte(bitsPerSample), byte(bitsPerSample >> 8),
		'd', 'a', 't', 'a',
		byte(dataSize), byte(dataSize >> 8), byte(dataSize >> 16), byte(dataSize >> 24),
	}

	if _, err := file.Write(header); err != nil {
		return fmt.Errorf("write wav header: %w", err)
	}

	if _, err := file.Write(pcmData); err != nil {
		return fmt.Errorf("write wav data: %w", err)
	}

	return nil
}
