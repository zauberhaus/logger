package syslog

import (
	"crypto/tls"
	"time"

	"go.uber.org/zap/zapcore"

	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

// Option configures a Sink.
type Option func(*Sink)

// WithNetwork sets the transport: "udp", "tcp" or "tcp+tls". Defaults to "udp".
func WithNetwork(network string) Option {
	return func(s *Sink) { s.network = network }
}

// WithTLSConfig enables TLS over TCP using the given configuration. It
// overrides WithNetwork.
func WithTLSConfig(cfg *tls.Config) Option {
	return func(s *Sink) {
		s.network = NetworkTLS
		s.tlsConfig = cfg
	}
}

// WithFacility sets the syslog facility. Defaults to FacilityUser.
func WithFacility(f Facility) Option {
	return func(s *Sink) { s.facility = f }
}

// WithAppName sets the RFC 5424 APP-NAME field. Defaults to the executable name.
func WithAppName(name string) Option {
	return func(s *Sink) { s.appName = name }
}

// WithHostname sets the RFC 5424 HOSTNAME field. Defaults to os.Hostname.
func WithHostname(host string) Option {
	return func(s *Sink) { s.hostname = host }
}

// WithDefaultSeverity sets the severity used for lines whose level cannot be
// determined, e.g. non-JSON output. Defaults to SeverityInfo.
func WithDefaultSeverity(sev Severity) Option {
	return func(s *Sink) { s.defaultSeverity = sev }
}

// WithDialTimeout sets the connect and write timeout. Defaults to 5s.
func WithDialTimeout(d time.Duration) Option {
	return func(s *Sink) { s.timeout = d }
}

// WithErrorHandler registers a callback invoked on delivery errors. When set,
// Write swallows delivery errors instead of returning them.
func WithErrorHandler(fn func(error)) Option {
	return func(s *Sink) { s.onError = fn }
}

// WithSyslog returns a zap.Option that forwards logs to a remote syslog
// server. Pair with zap.WithOutput(zap.JSONOutput) so the severity can be
// derived from the entry level.
func WithSyslog(address string, opts ...Option) (zaplogger.Option, error) {
	s, err := NewSink(address, opts...)
	if err != nil {
		return nil, err
	}
	return zaplogger.WithWriteSyncer(zapcore.AddSync(s)), nil
}
