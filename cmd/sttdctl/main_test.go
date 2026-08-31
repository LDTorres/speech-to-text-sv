package main

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/LDTorres/speech-to-text-sv/internal/modules/control"
)

func TestResponseErrorIncludesCodeAndMessage(t *testing.T) {
	err := responseError(control.Response{
		ErrorCode: control.ErrorCodeBusy,
		Message:   "session is processing",
	})

	if got, want := err.Error(), "busy: session is processing"; got != want {
		t.Fatalf("responseError() = %q, want %q", got, want)
	}
}

func TestParseBoolFlagAcceptsMixedCase(t *testing.T) {
	got, err := parseBoolFlag(" TRUE ")
	if err != nil {
		t.Fatalf("parseBoolFlag() returned error: %v", err)
	}
	if !got {
		t.Fatal("parseBoolFlag() returned false, want true")
	}
}

func TestSendControlRequestTimesOutWhenServerDoesNotRespond(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "control.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			defer func() { _ = conn.Close() }()
			select {
			case <-time.After(time.Second):
			case <-serverDone:
			}
		}
	}()
	defer func() { <-serverDone }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = sendControlRequest(ctx, socketPath, control.Request{Command: control.CommandPing})
	if err == nil {
		t.Fatal("sendControlRequest() returned nil, want timeout error")
	}
}
