package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
)

const (
	defaultEndpoint      = "https://log-api.newrelic.com/log/v1"
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	maxPayloadBytes      = 1 * 1024 * 1024 // NR uncompressed limit is 1MB per request
)

// Writer buffers log lines and ships them to the New Relic Log API.
// Use with zap.WithWriteSyncer or the WithNewRelic convenience option.
type Sink struct {
	licenseKey    string
	url           string
	service       string
	host          string
	batchSize     int
	flushInterval time.Duration
	attrs         map[string]string
	onError       func(error)

	mu    sync.Mutex
	lines [][]byte
	size  int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	client *http.Client
}

// NewWriter creates a Writer that forwards logs to the New Relic Log API.
// It performs a health check against the endpoint before returning; an error
// is returned if the license key is rejected or the endpoint is unreachable.
// Call Close when done to flush pending logs and stop the background flusher.
func NewSink(licenseKey string, opts ...Option) (logger.Sink, error) {
	w := &Sink{
		licenseKey:    licenseKey,
		url:           defaultEndpoint,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		client:        &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt.apply(w)
	}

	if err := w.healthCheck(); err != nil {
		return nil, err
	}

	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.done = make(chan struct{})
	go w.run()

	return w, nil
}

func (w *Sink) healthCheck() error {
	payload := []byte(`[{"common":{},"logs":[{"message":"health check"}]}]`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("newrelic: health check: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", w.licenseKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("newrelic: health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("newrelic: health check: HTTP %d", resp.StatusCode)
	}
	return nil
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
	flush := len(w.lines) >= w.batchSize || w.size >= maxPayloadBytes
	w.mu.Unlock()

	if flush {
		err := w.flush()
		if err != nil {
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

	payload := w.buildPayload(lines)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return w.handleError(fmt.Errorf("newrelic: build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", w.licenseKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return w.handleError(fmt.Errorf("newrelic: send logs: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return w.handleError(fmt.Errorf("newrelic: HTTP %d", resp.StatusCode))
	}

	return nil
}

// buildPayload constructs the New Relic Log API batch payload.
// Common attributes (service, hostname, extras) are hoisted into the "common"
// block so they are not repeated per entry. Each zap JSON log object is placed
// in the "logs" array; plain-text lines are wrapped with a "message" key.
func (w *Sink) buildPayload(lines [][]byte) []byte {
	var logsBuf bytes.Buffer
	logsBuf.WriteByte('[')
	for i, line := range lines {
		if i > 0 {
			logsBuf.WriteByte(',')
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 1 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' {
			logsBuf.Write(trimmed)
		} else {
			logsBuf.WriteString(`{"message":`)
			msgJSON, _ := json.Marshal(string(trimmed))
			logsBuf.Write(msgJSON)
			logsBuf.WriteByte('}')
		}
	}
	logsBuf.WriteByte(']')

	var commonBuf bytes.Buffer
	commonBuf.WriteByte('{')
	first := true
	writeAttr := func(key, val string) {
		if val == "" {
			return
		}
		if !first {
			commonBuf.WriteByte(',')
		}
		first = false
		kj, _ := json.Marshal(key)
		vj, _ := json.Marshal(val)
		commonBuf.Write(kj)
		commonBuf.WriteByte(':')
		commonBuf.Write(vj)
	}
	writeAttr("service.name", w.service)
	writeAttr("hostname", w.host)
	for k, v := range w.attrs {
		writeAttr(k, v)
	}
	commonBuf.WriteByte('}')

	var buf bytes.Buffer
	buf.WriteString(`[{"common":{"attributes":`)
	buf.Write(commonBuf.Bytes())
	buf.WriteString(`},"logs":`)
	buf.Write(logsBuf.Bytes())
	buf.WriteString(`}]`)
	return buf.Bytes()
}

func (w *Sink) handleError(err error) error {
	if w.onError != nil {
		w.onError(err)
		return nil
	}

	return err
}
