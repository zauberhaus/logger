package datadog

import (
	"net/http"
	"time"

	"go.uber.org/zap/zapcore"

	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

// Option configures a Sink.
type Option interface {
	apply(*Sink)
}

type optionFunc func(*Sink)

func (f optionFunc) apply(w *Sink) { f(w) }

// WithService sets the service name tag sent with every log entry.
func WithService(service string) Option {
	return optionFunc(func(w *Sink) { w.service = service })
}

// WithHost sets the hostname tag sent with every log entry.
func WithHost(host string) Option {
	return optionFunc(func(w *Sink) { w.host = host })
}

func WithSource(source string) Option {
	return optionFunc(func(w *Sink) { w.source = source })
}

// WithTags sets additional ddtags (comma-separated, e.g. "env:prod,region:us-east-1").
func WithTags(tags string) Option {
	return optionFunc(func(w *Sink) { w.tags = tags })
}

// WithBatchSize sets the maximum number of log lines per HTTP request. Defaults to 100.
func WithBatchSize(n int) Option {
	return optionFunc(func(w *Sink) { w.batchSize = n })
}

// WithFlushInterval sets how often buffered logs are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) Option {
	return optionFunc(func(w *Sink) { w.flushInterval = d })
}

// WithHTTPClient sets the HTTP client passed to the Datadog SDK.
// Primarily useful in tests to intercept or redirect SDK requests.
func WithHTTPClient(client *http.Client) Option {
	return optionFunc(func(w *Sink) { w.httpClient = client })
}

// WithErrorHandler registers a callback invoked on delivery errors (e.g. network failures).
func WithErrorHandler(fn func(error)) Option {
	return optionFunc(func(w *Sink) { w.onError = fn })
}

// WithDatadog returns a zap.Option that forwards logs to Datadog's log intake API.
// Pair with zap.WithOutput(zap.JSONOutput) to send structured log fields.
// Returns an error if the API key validation fails.
func WithDatadog(apiKey string, opts ...Option) (zaplogger.Option, error) {
	w, err := NewSink(apiKey, opts...)
	if err != nil {
		return nil, err
	}
	return zaplogger.WithWriteSyncer(zapcore.AddSync(w)), nil
}
