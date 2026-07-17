package control

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestServer_PingReturnsStatus(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("/tmp", "sttd-control-test-")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	socketPath := filepath.Join(tempDir, "control.sock")
	handler := &stubSessionService{
		status: session.Status{
			State:          session.StateIdle,
			RetryAvailable: true,
		},
	}
	server, err := NewServer(zap.NewNop(), socketPath, handler)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("unix socket bind not permitted in this environment: %v", err)
		}
		require.NoError(t, err)
	}
	defer func() {
		require.NoError(t, server.Stop(context.Background()))
	}()

	response := sendRequest(t, server.SocketPath(), Request{Command: CommandPing, Source: "test"})
	require.True(t, response.OK)
	require.Equal(t, session.StateIdle, response.State)
	require.True(t, response.RetryAvailable)
}

func sendRequest(t *testing.T, socketPath string, request Request) Response {
	t.Helper()

	conn, err := net.DialTimeout("unix", socketPath, time.Second)
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	_, err = conn.Write(encoded)
	require.NoError(t, err)

	if unixConn, ok := conn.(*net.UnixConn); ok {
		require.NoError(t, unixConn.CloseWrite())
	}

	var response Response
	require.NoError(t, json.NewDecoder(conn).Decode(&response))
	return response
}

type stubSessionService struct {
	status session.Status
}

func (s *stubSessionService) StartRecording(ctx context.Context) error {
	s.status.State = session.StateRecording
	return nil
}

func (s *stubSessionService) StopRecordingAndProcess(ctx context.Context) error {
	s.status.State = session.StateIdle
	return nil
}

func (s *stubSessionService) ToggleRecording(ctx context.Context) error {
	if s.status.State == session.StateRecording {
		s.status.State = session.StateIdle
		return nil
	}

	s.status.State = session.StateRecording
	return nil
}

func (s *stubSessionService) RetryLastPaste(ctx context.Context) error {
	return nil
}

func (s *stubSessionService) Status(ctx context.Context) session.Status {
	return s.status
}
