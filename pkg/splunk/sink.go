package splunk

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
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	defaultSourcetype    = "_json"
	maxPayloadBytes      = 5 * 1024 * 1024
)

// Writer buffers log lines and ships them to a Splunk HTTP Event Collector endpoint.
// Use with zap.WithWriteSyncer or the WithSplunk convenience option.
type Sink struct {
	url           string
	token         string
	source        string
	sourcetype    string
	index         string
	host          string
	batchSize     int
	flushInterval time.Duration
	onError       func(error)

	mu    sync.Mutex
	lines [][]byte
	size  int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	client *http.Client
}

// NewWriter creates a Writer that forwards logs to a Splunk HEC endpoint.
// It performs a health check against the endpoint before returning; an error
// is returned if the token is rejected or the endpoint is unreachable.
// Call Close when done to flush pending logs and stop the background flusher.
func NewSink(hecURL string, token string, opts ...Option) (logger.Sink, error) {
	w := &Sink{
		url:           hecURL,
		token:         token,
		sourcetype:    defaultSourcetype,
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
	payload := []byte(`{"event":"health check"}`)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, w.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("splunk: health check: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Splunk "+w.token)

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("splunk: health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("splunk: health check: HTTP %d", resp.StatusCode)
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
		return w.handleError(fmt.Errorf("splunk: build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Splunk "+w.token)

	resp, err := w.client.Do(req)
	if err != nil {
		return w.handleError(fmt.Errorf("splunk: send logs: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return w.handleError(fmt.Errorf("splunk: HTTP %d", resp.StatusCode))
	}

	return nil
}

// buildPayload constructs newline-delimited Splunk HEC events.
// For JSON log lines produced by zap JSONOutput, the event field is a raw JSON
// object; otherwise the line is encoded as a JSON string.
func (w *Sink) buildPayload(lines [][]byte) []byte {
	var buf bytes.Buffer
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		buf.WriteString(`{"event":`)
		if len(trimmed) > 1 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}' {
			buf.Write(trimmed)
		} else {
			eventJSON, _ := json.Marshal(string(trimmed))
			buf.Write(eventJSON)
		}
		w.appendHECFields(&buf)
		buf.WriteString("}\n")
	}
	return buf.Bytes()
}

func (w *Sink) appendHECFields(buf *bytes.Buffer) {
	writeJSONField(buf, "source", w.source)
	writeJSONField(buf, "sourcetype", w.sourcetype)
	writeJSONField(buf, "index", w.index)
	writeJSONField(buf, "host", w.host)
}

func (w *Sink) handleError(err error) error {
	if w.onError != nil {
		w.onError(err)
		return nil
	}

	return err
}

func writeJSONField(buf *bytes.Buffer, key, val string) {
	if val == "" {
		return
	}
	buf.WriteByte(',')
	keyJSON, _ := json.Marshal(key)
	buf.Write(keyJSON)
	buf.WriteByte(':')
	valJSON, _ := json.Marshal(val)
	buf.Write(valJSON)
}
