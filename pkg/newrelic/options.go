package newrelic

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

// WithRegion sets the New Relic region. Use "EU" for the EU data center;
// any other value targets the default US endpoint.
func WithRegion(region string) Option {
	return optionFunc(func(w *Sink) {
		if region == "EU" {
			w.url = "https://log-api.eu.newrelic.com/log/v1"
		} else {
			w.url = defaultEndpoint
		}
	})
}

// WithURL overrides the full Log API URL. Useful for testing or proxies.
func WithURL(url string) Option {
	return optionFunc(func(w *Sink) { w.url = url })
}

// WithService sets the service.name attribute sent with every log batch.
func WithService(service string) Option {
	return optionFunc(func(w *Sink) { w.service = service })
}

// WithHost sets the hostname attribute sent with every log batch.
func WithHost(host string) Option {
	return optionFunc(func(w *Sink) { w.host = host })
}

// WithAttribute adds a custom common attribute sent with every log batch.
func WithAttribute(key, value string) Option {
	return optionFunc(func(w *Sink) {
		if w.attrs == nil {
			w.attrs = make(map[string]string)
		}
		w.attrs[key] = value
	})
}

// WithBatchSize sets the maximum number of log lines per HTTP request. Defaults to 100.
func WithBatchSize(n int) Option {
	return optionFunc(func(w *Sink) { w.batchSize = n })
}

// WithFlushInterval sets how often buffered logs are flushed. Defaults to 5s.
func WithFlushInterval(d time.Duration) Option {
	return optionFunc(func(w *Sink) { w.flushInterval = d })
}

// WithHTTPClient sets the HTTP client used for log ingestion requests.
func WithHTTPClient(client *http.Client) Option {
	return optionFunc(func(w *Sink) { w.client = client })
}

// WithErrorHandler registers a callback invoked on delivery errors.
func WithErrorHandler(fn func(error)) Option {
	return optionFunc(func(w *Sink) { w.onError = fn })
}

func WithNewRelic(licenseKey string, opts ...Option) (zaplogger.Option, error) {
	w, err := NewSink(licenseKey, opts...)
	if err != nil {
		return nil, err
	}

	return zaplogger.WithWriteSyncer(zapcore.AddSync(w)), nil
}
