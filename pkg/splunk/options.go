package splunk

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

// WithSource sets the Splunk source field.
func WithSource(source string) Option {
	return optionFunc(func(w *Sink) { w.source = source })
}

// WithSourcetype sets the Splunk sourcetype field. Defaults to "_json".
func WithSourcetype(sourcetype string) Option {
	return optionFunc(func(w *Sink) { w.sourcetype = sourcetype })
}

// WithIndex sets the Splunk index field.
func WithIndex(index string) Option {
	return optionFunc(func(w *Sink) { w.index = index })
}

// WithHost sets the Splunk host field.
func WithHost(host string) Option {
	return optionFunc(func(w *Sink) { w.host = host })
}

// WithBatchSize sets the maximum number of log events per HTTP request. Defaults to 100.
func WithBatchSize(n int) Option {
	return optionFunc(func(w *Sink) { w.batchSize = n })
}

// WithFlushInterval sets how often buffered logs are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) Option {
	return optionFunc(func(w *Sink) { w.flushInterval = d })
}

// WithHTTPClient sets the HTTP client used for HEC requests.
func WithHTTPClient(client *http.Client) Option {
	return optionFunc(func(w *Sink) { w.client = client })
}

// WithErrorHandler registers a callback invoked on delivery errors (e.g. network failures).
func WithErrorHandler(fn func(error)) Option {
	return optionFunc(func(w *Sink) { w.onError = fn })
}

// WithSplunk returns a zap.Option that forwards logs to a Splunk HEC endpoint.
// Pair with zap.WithOutput(zap.JSONOutput) to send structured log fields.
func WithSplunk(hecURL, token string, opts ...Option) (zaplogger.Option, error) {
	w, err := NewSink(hecURL, token, opts...)
	if err != nil {
		return nil, err
	}
	return zaplogger.WithWriteSyncer(zapcore.AddSync(w)), nil
}
