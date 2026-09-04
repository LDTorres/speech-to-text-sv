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
	"time"

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

const (
	maxRequestBytes       = 64 * 1024
	maxConcurrentRequests = 16
	requestTimeout        = 2 * time.Minute
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

	mu          sync.Mutex
	listener    net.Listener
	done        chan struct{}
	cancel      context.CancelFunc
	connections map[net.Conn]struct{}
	requests    chan struct{}
	handlers    sync.WaitGroup
}

func NewServer(logger *zap.Logger, socketPath string, handler session.Service) (*Server, error) {
	resolvedPath, err := ResolveSocketPath(socketPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		logger:      logger,
		socketPath:  resolvedPath,
		handler:     handler,
		connections: make(map[net.Conn]struct{}),
		requests:    make(chan struct{}, maxConcurrentRequests),
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

	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	if info, err := os.Lstat(s.socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("socket path is not a unix socket: %q", s.socketPath)
		}
		if err := os.Remove(s.socketPath); err != nil {
			return fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect stale socket: %w", err)
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

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.listener = listener
	s.done = done
	s.cancel = cancel

	go s.run(runCtx, listener, done)

	s.logger.Info("external control started", zap.String("socket_path", s.socketPath))

	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	listener := s.listener
	done := s.done
	cancel := s.cancel
	s.mu.Unlock()

	if listener == nil {
		return nil
	}

	if cancel != nil {
		cancel()
	}
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("close listener: %w", err)
	}
	s.closeConnections()

	select {
	case <-done:
		s.mu.Lock()
		if s.listener == listener {
			s.listener = nil
			s.done = nil
			s.cancel = nil
		}
		s.mu.Unlock()
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

	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = listener.Close()
			s.closeConnections()
		case <-watchDone:
		}
	}()
	defer close(watchDone)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				break
			}
			s.logger.Warn("accept external control connection", zap.Error(err))
			continue
		}

		select {
		case s.requests <- struct{}{}:
			s.handlers.Add(1)
			s.mu.Lock()
			s.connections[conn] = struct{}{}
			s.mu.Unlock()
			go func() {
				defer s.handlers.Done()
				defer func() { <-s.requests }()
				defer s.removeConnection(conn)
				s.handleConn(ctx, conn)
			}()
		default:
			_ = conn.Close()
		}
	}

	s.closeConnections()
	s.handlers.Wait()
}

func (s *Server) closeConnections() {
	s.mu.Lock()
	connections := make([]net.Conn, 0, len(s.connections))
	for conn := range s.connections {
		connections = append(connections, conn)
	}
	s.mu.Unlock()

	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (s *Server) removeConnection(conn net.Conn) {
	s.mu.Lock()
	delete(s.connections, conn)
	s.mu.Unlock()
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()

	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	data, err := io.ReadAll(io.LimitReader(conn, maxRequestBytes+1))
	if err != nil {
		s.writeResponse(conn, Response{
			OK:        false,
			ErrorCode: ErrorCodeInternal,
			Message:   fmt.Sprintf("read request: %v", err),
		})
		return
	}
	if len(data) > maxRequestBytes {
		s.writeResponse(conn, Response{
			OK:        false,
			ErrorCode: ErrorCodeInternal,
			Message:   "request exceeds maximum size",
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
		return s.statusResponse(ctx, true, "")
	case CommandStart:
		if err := s.handler.StartRecording(ctx); err != nil {
			return s.errorResponse(ctx, err)
		}
		return s.statusResponse(ctx, true, "recording started")
	case CommandStop:
		if err := s.handler.StopRecordingAndProcess(ctx); err != nil {
			return s.errorResponse(ctx, err)
		}
		return s.statusResponse(ctx, true, "recording stopped")
	case CommandToggle:
		if err := s.handler.ToggleRecording(ctx); err != nil {
			return s.errorResponse(ctx, err)
		}
		return s.statusResponse(ctx, true, "recording toggled")
	case CommandRetry:
		if err := s.handler.RetryLastPaste(ctx); err != nil {
			return s.errorResponse(ctx, err)
		}
		return s.statusResponse(ctx, true, "retry completed")
	default:
		return Response{
			OK:        false,
			ErrorCode: ErrorCodeInvalidState,
			Message:   fmt.Sprintf("unsupported command %q", request.Command),
		}
	}
}

func (s *Server) statusResponse(ctx context.Context, ok bool, message string) Response {
	status := s.handler.Status(ctx)
	return Response{
		OK:             ok,
		State:          status.State,
		RetryAvailable: status.RetryAvailable,
		Message:        message,
	}
}

func (s *Server) errorResponse(ctx context.Context, err error) Response {
	status := s.handler.Status(ctx)
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
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		if !filepath.IsAbs(trimmed) {
			return "", errors.New("external control socket path must be absolute")
		}
		return filepath.Clean(trimmed), nil
	}

	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		return "", errors.New("external control requires STTD_EXTERNAL_CONTROL_SOCKET_PATH or XDG_RUNTIME_DIR")
	}
	if !filepath.IsAbs(runtimeDir) {
		return "", errors.New("XDG_RUNTIME_DIR must be absolute")
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
