package papertrail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
)

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	defaultQueueSize     = 10000
	defaultTimeout       = 10 * time.Second
	maxPayloadBytes      = 1 * 1024 * 1024
)

var (
	ErrQueueFull = errors.New("papertrail: queue full, log line dropped")
	ErrClosed    = errors.New("papertrail: sink closed")
)

// Sink ships logs to the SolarWinds Papertrail HTTP ingestion API
// (application/octet-stream, Bearer auth).
//
// Write never blocks on the network: lines go into a bounded input queue and
// a background goroutine batches and delivers them. When the queue is full,
// new lines are dropped and ErrQueueFull is reported.
type Sink struct {
	endpoint   string
	token      string
	httpClient *http.Client
	onError    func(error)
	allowHTTP  bool

	batchSize     int
	flushInterval time.Duration
	queueSize     int

	mu     sync.RWMutex
	closed bool

	queue     chan []byte
	syncReq   chan chan error
	stop      chan struct{}
	done      chan struct{}
	closeErr  error
	closeOnce sync.Once

	// Owned by the run goroutine.
	lines [][]byte
	size  int
}

func NewSink(endpoint, token string, opts ...SinkOption) (logger.Sink, error) {
	w := &Sink{
		endpoint:      endpoint,
		token:         token,
		httpClient:    &http.Client{Timeout: defaultTimeout},
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		queueSize:     defaultQueueSize,
	}
	for _, opt := range opts {
		opt(w)
	}

	if err := logger.CheckEndpoint(w.endpoint, w.allowHTTP); err != nil {
		return nil, fmt.Errorf("papertrail: %w", err)
	}
	w.httpClient = logger.SecureClient(w.httpClient, w.allowHTTP)

	if err := w.healthCheck(); err != nil {
		return nil, fmt.Errorf("papertrail: health check: %w", err)
	}

	w.queue = make(chan []byte, w.queueSize)
	w.syncReq = make(chan chan error)
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	go w.run()

	return w, nil
}

// Write enqueues a log line without waiting for delivery. Implements io.Writer
// and zapcore.WriteSyncer.
func (w *Sink) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\n")
	if len(line) == 0 {
		return len(p), nil
	}
	dst := make([]byte, len(line))
	copy(dst, line)

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return 0, w.handleError(ErrClosed)
	}

	select {
	case w.queue <- dst:
		return len(p), nil
	default:
		return 0, w.handleError(ErrQueueFull)
	}
}

// Sync delivers all lines written before the call and waits for the result.
// Implements zapcore.WriteSyncer.
func (w *Sink) Sync() error {
	reply := make(chan error, 1)
	select {
	case w.syncReq <- reply:
		return <-reply
	case <-w.done:
		return nil
	}
}

// Close stops accepting lines, delivers everything still queued and stops
// the background worker.
func (w *Sink) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		w.mu.Unlock()

		close(w.stop)
		<-w.done
	})
	return w.closeErr
}

func (w *Sink) run() {
	defer close(w.done)

	t := time.NewTicker(w.flushInterval)
	defer t.Stop()

	for {
		select {
		case line := <-w.queue:
			w.add(line)
		case <-t.C:
			w.flush()
		case reply := <-w.syncReq:
			w.drain()
			reply <- w.flush()
		case <-w.stop:
			// Close holds no writers past this point, so the queue is final.
			w.drain()
			w.closeErr = w.flush()
			return
		}
	}
}

// add appends a line to the current batch and flushes when it is full.
func (w *Sink) add(line []byte) {
	w.lines = append(w.lines, line)
	w.size += len(line)
	if len(w.lines) >= w.batchSize || w.size >= maxPayloadBytes {
		w.flush()
	}
}

// drain moves every line currently in the queue into batches.
func (w *Sink) drain() {
	for n := len(w.queue); n > 0; n-- {
		w.add(<-w.queue)
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

// flush sends the current batch. Must only be called from the run goroutine.
func (w *Sink) flush() error {
	if len(w.lines) == 0 {
		return nil
	}
	lines := w.lines
	w.lines = nil
	w.size = 0

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
