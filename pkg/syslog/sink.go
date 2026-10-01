// cspell:words UUCP
package syslog

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
)

// Facility is a syslog facility code (RFC 5424, section 6.2.1).
type Facility int

const (
	FacilityKern Facility = iota
	FacilityUser
	FacilityMail
	FacilityDaemon
	FacilityAuth
	FacilitySyslog
	FacilityLPR
	FacilityNews
	FacilityUUCP
	FacilityCron
	FacilityAuthPriv
	FacilityFTP
)

// Local use facilities.
const (
	FacilityLocal0 Facility = 16 + iota
	FacilityLocal1
	FacilityLocal2
	FacilityLocal3
	FacilityLocal4
	FacilityLocal5
	FacilityLocal6
	FacilityLocal7
)

// Severity is a syslog severity code (RFC 5424, section 6.2.1).
type Severity int

const (
	SeverityEmergency Severity = iota
	SeverityAlert
	SeverityCritical
	SeverityError
	SeverityWarning
	SeverityNotice
	SeverityInfo
	SeverityDebug
)

// Supported values for WithNetwork.
const (
	NetworkUDP = "udp"
	NetworkTCP = "tcp"
	NetworkTLS = "tcp+tls"
)

const defaultTimeout = 5 * time.Second

// Sink ships log lines to a remote syslog server as RFC 5424 messages.
// Over TCP and TLS, messages are newline-delimited (RFC 6587 non-transparent
// framing) and the connection is re-established after a failed write.
type Sink struct {
	address   string
	network   string
	tlsConfig *tls.Config
	timeout   time.Duration
	onError   func(error)

	facility        Facility
	defaultSeverity Severity
	appName         string
	hostname        string
	procID          string

	mu     sync.Mutex
	conn   net.Conn
	closed bool
}

// NewSink creates a Sink that sends logs to the syslog server at address
// ("host:port"). It connects immediately and returns an error if the server
// cannot be reached.
func NewSink(address string, opts ...Option) (logger.Sink, error) {
	s := &Sink{
		address:         address,
		network:         NetworkUDP,
		timeout:         defaultTimeout,
		facility:        FacilityUser,
		defaultSeverity: SeverityInfo,
		procID:          strconv.Itoa(os.Getpid()),
	}
	for _, opt := range opts {
		opt(s)
	}

	switch s.network {
	case NetworkUDP, NetworkTCP, NetworkTLS:
	default:
		return nil, fmt.Errorf("syslog: unsupported network %q", s.network)
	}

	if s.appName == "" {
		s.appName = filepath.Base(os.Args[0])
	}
	if s.hostname == "" {
		s.hostname, _ = os.Hostname()
	}

	if err := s.connect(); err != nil {
		return nil, fmt.Errorf("syslog: connect: %w", err)
	}

	return s, nil
}

// Write sends p as a single syslog message. Implements io.Writer and
// zapcore.WriteSyncer.
func (s *Sink) Write(p []byte) (int, error) {
	line := bytes.TrimRight(p, "\r\n")
	if len(line) == 0 {
		return len(p), nil
	}

	msg := s.format(line, time.Now())

	s.mu.Lock()
	err := s.send(msg)
	s.mu.Unlock()

	if err != nil {
		if err = s.handleError(fmt.Errorf("syslog: send log: %w", err)); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Sync is a no-op because messages are sent synchronously.
func (s *Sink) Sync() error {
	return nil
}

// Close closes the connection to the syslog server.
func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.conn == nil {
		return nil
	}

	err := s.conn.Close()
	s.conn = nil
	return err
}

func (s *Sink) connect() error {
	dialer := &net.Dialer{Timeout: s.timeout}

	var (
		conn net.Conn
		err  error
	)
	switch s.network {
	case NetworkTLS:
		conn, err = tls.DialWithDialer(dialer, "tcp", s.address, s.tlsConfig)
	default:
		conn, err = dialer.Dial(s.network, s.address)
	}
	if err != nil {
		return err
	}

	s.conn = conn
	return nil
}

// send writes msg, reconnecting once if the connection was lost.
// The caller must hold s.mu.
func (s *Sink) send(msg []byte) error {
	if s.closed {
		return net.ErrClosed
	}

	if s.conn != nil {
		if err := s.write(msg); err == nil {
			return nil
		}
		s.conn.Close()
		s.conn = nil
	}

	if err := s.connect(); err != nil {
		return err
	}
	if err := s.write(msg); err != nil {
		s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

func (s *Sink) write(msg []byte) error {
	if err := s.conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		return err
	}
	_, err := s.conn.Write(msg)
	return err
}

// format builds an RFC 5424 message:
//
//	<PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
//
// Over TCP the message is terminated by a newline.
func (s *Sink) format(line []byte, ts time.Time) []byte {
	pri := int(s.facility)*8 + int(s.severity(line))

	var b bytes.Buffer
	b.Grow(len(line) + 96)
	fmt.Fprintf(&b, "<%d>1 %s %s %s %s - - ",
		pri,
		ts.Format(time.RFC3339Nano),
		field(s.hostname, 255),
		field(s.appName, 48),
		field(s.procID, 128),
	)
	b.Write(line)
	if s.network != NetworkUDP {
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// severity derives the severity from the level of a zap JSON line. Lines
// that are not JSON, or carry no known level, get the default severity.
func (s *Sink) severity(line []byte) Severity {
	if len(line) == 0 || line[0] != '{' {
		return s.defaultSeverity
	}

	_, rest, ok := bytes.Cut(line, []byte(`"level":"`))
	if !ok {
		return s.defaultSeverity
	}
	level, _, ok := bytes.Cut(rest, []byte(`"`))
	if !ok {
		return s.defaultSeverity
	}

	switch string(level) {
	case "debug":
		return SeverityDebug
	case "info":
		return SeverityInfo
	case "warn":
		return SeverityWarning
	case "error":
		return SeverityError
	case "dpanic":
		return SeverityCritical
	case "panic":
		return SeverityAlert
	case "fatal":
		return SeverityEmergency
	default:
		return s.defaultSeverity
	}
}

func (s *Sink) handleError(err error) error {
	if s.onError != nil {
		s.onError(err)
		return nil
	}
	return err
}

// field sanitizes an RFC 5424 header field: printable US-ASCII without
// spaces, at most max bytes, or "-" when empty.
func field(v string, max int) string {
	b := make([]byte, 0, len(v))
	for i := 0; i < len(v) && len(b) < max; i++ {
		if c := v[i]; c > 32 && c < 127 {
			b = append(b, c)
		}
	}
	if len(b) == 0 {
		return "-"
	}
	return string(b)
}
