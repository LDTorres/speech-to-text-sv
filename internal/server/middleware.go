package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

type timeoutWriter struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
	mu          sync.Mutex
}

type responseError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type envelope struct {
	Data  any            `json:"data"`
	Error *responseError `json:"error"`
}

type timeoutResult struct {
	recovered any
}

func (w *statusWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}

	w.status = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(data)
}

func newTimeoutWriter() *timeoutWriter {
	return &timeoutWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (w *timeoutWriter) Header() http.Header {
	return w.header
}

func (w *timeoutWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wroteHeader {
		return
	}

	w.status = statusCode
	w.wroteHeader = true
}

func (w *timeoutWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}

	return w.body.Write(data)
}

func (w *timeoutWriter) WriteTo(dst http.ResponseWriter) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}

	dst.WriteHeader(w.status)

	_, err := dst.Write(w.body.Bytes())
	return err
}

func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(writer, r)

			log.Info(
				"http request completed",
				zap.String("request_id", middleware.GetReqID(r.Context())),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", writer.status),
				zap.Duration("duration", time.Since(startedAt)),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

func Recoverer(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Error(
						"panic recovered",
						zap.Any("panic", recovered),
						zap.ByteString("stack", debug.Stack()),
						zap.String("request_id", middleware.GetReqID(r.Context())),
						zap.String("path", r.URL.Path),
					)

					writeJSON(w, http.StatusInternalServerError, map[string]string{
						"error": "internal server error",
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			bufferedWriter := newTimeoutWriter()
			resultChannel := make(chan timeoutResult, 1)

			go func() {
				result := timeoutResult{}
				defer func() {
					if recovered := recover(); recovered != nil {
						result.recovered = recovered
					}

					resultChannel <- result
				}()

				next.ServeHTTP(bufferedWriter, r.WithContext(ctx))
			}()

			select {
			case result := <-resultChannel:
				if result.recovered != nil {
					panic(result.recovered)
				}

				if err := bufferedWriter.WriteTo(w); err != nil {
					writeJSON(w, http.StatusInternalServerError, envelope{
						Data: nil,
						Error: &responseError{
							Message: "internal server error",
							Code:    "internal_error",
						},
					})
				}
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					writeJSON(w, http.StatusGatewayTimeout, envelope{
						Data: nil,
						Error: &responseError{
							Message: "request timed out",
							Code:    "request_timeout",
						},
					})
				}
			}
		})
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(`{"data":null,"error":{"message":"internal server error","code":"internal_error"}}`)
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(append(body, '\n'))
}
