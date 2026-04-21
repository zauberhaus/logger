package papertrail

import (
	"net/http"
	"time"
)

// SinkOption configures a Writer2.
type SinkOption func(*Sink)

// WithHTTPClient injects a custom HTTP client. Primarily useful in tests.
func WithHTTPClient(c *http.Client) SinkOption {
	return func(w *Sink) { w.httpClient = c }
}

// WithBatchSize sets the maximum number of lines per HTTP request. Defaults to 100.
func WithBatchSize(n int) SinkOption {
	return func(w *Sink) { w.batchSize = n }
}

// WithFlushInterval sets how often buffered lines are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) SinkOption {
	return func(w *Sink) { w.flushInterval = d }
}

// WithErrorHandler registers a callback invoked on delivery errors.
func WithErrorHandler(fn func(error)) SinkOption {
	return func(w *Sink) { w.onError = fn }
}
