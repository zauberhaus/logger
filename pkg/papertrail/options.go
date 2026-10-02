package papertrail

import (
	"net/http"
	"time"
)

// SinkOption configures a Sink.
type SinkOption func(*Sink)

// WithHTTPClient injects a custom HTTP client. Defaults to a client with a
// 10s timeout.
func WithHTTPClient(c *http.Client) SinkOption {
	return func(w *Sink) { w.httpClient = c }
}

// AllowHTTP permits a plain http endpoint, e.g. for local tests. By
// default only https endpoints are accepted.
func AllowHTTP() SinkOption {
	return func(w *Sink) { w.allowHTTP = true }
}

// WithBatchSize sets the maximum number of lines per HTTP request. Defaults to 100.
func WithBatchSize(n int) SinkOption {
	return func(w *Sink) { w.batchSize = n }
}

// WithFlushInterval sets how often buffered lines are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) SinkOption {
	return func(w *Sink) { w.flushInterval = d }
}

// WithQueueSize sets the capacity of the input queue in lines. When the queue
// is full, Write drops the line and reports ErrQueueFull. Defaults to 10000.
func WithQueueSize(n int) SinkOption {
	return func(w *Sink) { w.queueSize = n }
}

// WithErrorHandler registers a callback invoked on delivery errors, dropped
// lines and writes after Close. When set, Write and Sync swallow these errors
// instead of returning them. Errors from background deliveries are only
// visible through this handler. The callback may be called concurrently.
func WithErrorHandler(fn func(error)) SinkOption {
	return func(w *Sink) { w.onError = fn }
}
