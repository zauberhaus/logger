package papertrail

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
)

const (
	defaultBatchSize2     = 100
	defaultFlushInterval2 = 5 * time.Second
	maxPayloadBytes2      = 1 * 1024 * 1024
)

// Writer ships logs to the SolarWinds Papertrail HTTP ingestion API
// (application/octet-stream, Bearer auth).
type Sink struct {
	endpoint   string
	token      string
	httpClient *http.Client
	onError    func(error)

	batchSize     int
	flushInterval time.Duration

	mu    sync.Mutex
	lines [][]byte
	size  int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func NewSink(endpoint, token string, opts ...SinkOption) (logger.Sink, error) {
	w := &Sink{
		endpoint:      endpoint,
		token:         token,
		httpClient:    http.DefaultClient,
		batchSize:     defaultBatchSize2,
		flushInterval: defaultFlushInterval2,
		done:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt(w)
	}

	if err := w.healthCheck(); err != nil {
		return nil, fmt.Errorf("papertrail: health check: %w", err)
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.run()
	return w, nil
}

// Write buffers a log line. Implements io.Writer and zapcore.WriteSyncer.
func (w *Sink) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\n")
	if len(line) == 0 {
		return len(p), nil
	}
	dst := make([]byte, len(line))
	copy(dst, line)

	w.mu.Lock()
	w.lines = append(w.lines, dst)
	w.size += len(dst)
	flush := len(w.lines) >= w.batchSize || w.size >= maxPayloadBytes2
	w.mu.Unlock()

	if flush {
		if err := w.flush(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Sync flushes buffered logs immediately. Implements zapcore.WriteSyncer.
func (w *Sink) Sync() error {
	return w.flush()
}

// Close flushes remaining logs and stops the background flusher.
func (w *Sink) Close() error {
	w.cancel()
	<-w.done
	return w.flush()
}

func (w *Sink) run() {
	defer close(w.done)
	t := time.NewTicker(w.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.flush()
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *Sink) healthCheck() error {
	req, err := http.NewRequest(http.MethodPost, w.endpoint, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+w.token)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("HTTP %d: invalid token", resp.StatusCode)
	}
	return nil
}

func (w *Sink) flush() error {
	w.mu.Lock()
	if len(w.lines) == 0 {
		w.mu.Unlock()
		return nil
	}
	lines := w.lines
	w.lines = nil
	w.size = 0
	w.mu.Unlock()

	body := bytes.Join(lines, []byte("\n"))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.endpoint, bytes.NewReader(body))
	if err != nil {
		return w.handleError(fmt.Errorf("papertrail: build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+w.token)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return w.handleError(fmt.Errorf("papertrail: send logs: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return w.handleError(fmt.Errorf("papertrail: HTTP %d", resp.StatusCode))
	}
	return nil
}

func (w *Sink) handleError(err error) error {
	if w.onError != nil {
		w.onError(err)
		return nil
	}
	return err
}
