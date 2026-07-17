package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/session"
	"go.uber.org/zap"
)

const (
	CommandPing   = "ping"
	CommandStatus = "status"
	CommandStart  = "start"
	CommandStop   = "stop"
	CommandToggle = "toggle"
	CommandRetry  = "retry"
)

const (
	ErrorCodeBusy         = "busy"
	ErrorCodeInvalidState = "invalid_state"
	ErrorCodeUnavailable  = "unavailable"
	ErrorCodeInternal     = "internal_error"
)

type Request struct {
	Command   string `json:"command"`
	Source    string `json:"source,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type Response struct {
	OK             bool          `json:"ok"`
	State          session.State `json:"state"`
	RetryAvailable bool          `json:"retry_available"`
	Message        string        `json:"message,omitempty"`
	ErrorCode      string        `json:"error_code,omitempty"`
}

type Server struct {
	logger     *zap.Logger
	socketPath string
	handler    session.Service

	mu       sync.Mutex
	listener net.Listener
	done     chan struct{}
}

func NewServer(logger *zap.Logger, socketPath string, handler session.Service) (*Server, error) {
	resolvedPath, err := ResolveSocketPath(socketPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		logger:     logger,
		socketPath: resolvedPath,
		handler:    handler,
	}, nil
}

func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return errors.New("external control already started")
	}

	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket %q: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(s.socketPath)
		return fmt.Errorf("chmod socket: %w", err)
	}

	done := make(chan struct{})
	s.listener = listener
	s.done = done

	go s.run(ctx, listener, done)

	s.logger.Info("external control started", zap.String("socket_path", s.socketPath))

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	listener := s.listener
	done := s.done
	s.listener = nil
	s.done = nil
	s.mu.Unlock()

	if listener == nil {
		return nil
	}

	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close listener: %w", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove socket: %w", err)
	}

	s.logger.Info("external control stopped", zap.String("socket_path", s.socketPath))

	return nil
}

func (s *Server) run(ctx context.Context, listener net.Listener, done chan struct{}) {
	defer close(done)
	defer func() {
		_ = listener.Close()
		_ = os.Remove(s.socketPath)
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}
			s.logger.Warn("accept external control connection", zap.Error(err))
			continue
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	data, err := io.ReadAll(conn)
	if err != nil {
		s.writeResponse(conn, Response{
			OK:        false,
			ErrorCode: ErrorCodeInternal,
			Message:   fmt.Sprintf("read request: %v", err),
		})
		return
	}

	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		s.writeResponse(conn, Response{
			OK:        false,
			ErrorCode: ErrorCodeInternal,
			Message:   fmt.Sprintf("invalid request: %v", err),
		})
		return
	}

	source := strings.TrimSpace(request.Source)
	if source == "" {
		source = "external"
	}

	s.logger.Info(
		"external control request",
		zap.String("command", request.Command),
		zap.String("source", source),
		zap.String("request_id", request.RequestID),
	)

	response := s.handleRequest(ctx, request)
	s.writeResponse(conn, response)
}

func (s *Server) handleRequest(ctx context.Context, request Request) Response {
	switch request.Command {
	case CommandPing, CommandStatus:
		return s.statusResponse(true, "")
	case CommandStart:
		if err := s.handler.StartRecording(ctx); err != nil {
			return s.errorResponse(err)
		}
		return s.statusResponse(true, "recording started")
	case CommandStop:
		if err := s.handler.StopRecordingAndProcess(ctx); err != nil {
			return s.errorResponse(err)
		}
		return s.statusResponse(true, "recording stopped")
	case CommandToggle:
		if err := s.handler.ToggleRecording(ctx); err != nil {
			return s.errorResponse(err)
		}
		return s.statusResponse(true, "recording toggled")
	case CommandRetry:
		if err := s.handler.RetryLastPaste(ctx); err != nil {
			return s.errorResponse(err)
		}
		return s.statusResponse(true, "retry completed")
	default:
		return Response{
			OK:        false,
			ErrorCode: ErrorCodeInvalidState,
			Message:   fmt.Sprintf("unsupported command %q", request.Command),
		}
	}
}

func (s *Server) statusResponse(ok bool, message string) Response {
	status := s.handler.Status(context.Background())
	return Response{
		OK:             ok,
		State:          status.State,
		RetryAvailable: status.RetryAvailable,
		Message:        message,
	}
}

func (s *Server) errorResponse(err error) Response {
	status := s.handler.Status(context.Background())
	code := mapErrorCode(err)

	return Response{
		OK:             false,
		State:          status.State,
		RetryAvailable: status.RetryAvailable,
		Message:        err.Error(),
		ErrorCode:      code,
	}
}

func (s *Server) writeResponse(conn net.Conn, response Response) {
	encoded, err := json.Marshal(response)
	if err != nil {
		s.logger.Error("marshal external control response", zap.Error(err))
		return
	}

	if _, err := conn.Write(encoded); err != nil {
		s.logger.Warn("write external control response", zap.Error(err))
	}
}

func ResolveSocketPath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}

	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		return "", errors.New("external control requires STTD_EXTERNAL_CONTROL_SOCKET_PATH or XDG_RUNTIME_DIR")
	}

	return filepath.Join(runtimeDir, "sttd", "control.sock"), nil
}

func mapErrorCode(err error) string {
	switch {
	case errors.Is(err, session.ErrBusy):
		return ErrorCodeBusy
	case errors.Is(err, session.ErrAlreadyRecording),
		errors.Is(err, session.ErrNotRecording),
		errors.Is(err, session.ErrNoTranscript),
		errors.Is(err, session.ErrInvalidState):
		return ErrorCodeInvalidState
	default:
		return ErrorCodeInternal
	}
}
