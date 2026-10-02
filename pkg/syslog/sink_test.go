// cspell:ignore fakehost hllo héllo myapp NILVALUE sctp tmpl
package syslog_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/syslog"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

func listenUDP(t *testing.T) (*net.UDPConn, string) {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return conn.(*net.UDPConn), conn.LocalAddr().String()
}

func readPacket(t *testing.T, conn *net.UDPConn) string {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 4096)
	n, _, err := conn.ReadFrom(buf)
	require.NoError(t, err)

	return string(buf[:n])
}

// acceptLines accepts connections on l and sends every received octet-counted
// frame to the returned channel. The returned function closes all accepted connections.
func acceptLines(l net.Listener) (<-chan string, func()) {
	var (
		mu    sync.Mutex
		conns []net.Conn
	)

	lines := make(chan string, 16)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}

			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()

			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					msg, err := readFrame(r)
					if err != nil {
						return
					}
					lines <- msg
				}
			}()
		}
	}()

	return lines, func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			c.Close()
		}
	}
}

// readFrame reads one RFC 6587 octet-counted frame: "<len> <msg>".
func readFrame(r *bufio.Reader) (string, error) {
	prefix, err := r.ReadString(' ')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSuffix(prefix, " "))
	if err != nil {
		return "", err
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func nextLine(t *testing.T, lines <-chan string) string {
	t.Helper()

	select {
	case l := <-lines:
		return l
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for syslog line")
		return ""
	}
}

func TestSink_UDP(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr,
		syslog.WithAppName("myapp"),
		syslog.WithHostname("myhost"),
		syslog.WithFacility(syslog.FacilityLocal0),
	)
	require.NoError(t, err)
	defer s.Close()

	n, err := s.Write([]byte("hello syslog\n"))
	require.NoError(t, err)
	assert.Equal(t, len("hello syslog\n"), n)

	// facility local0 (16) * 8 + info (6) = 134
	msg := readPacket(t, srv)
	assert.Regexp(t, `^<134>1 \d{4}-\d\d-\d\dT\S+ myhost myapp \d+ - - hello syslog$`, msg)
}

func TestSink_SeverityFromJSONLevel(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr, syslog.WithFacility(syslog.FacilityUser))
	require.NoError(t, err)
	defer s.Close()

	tests := []struct {
		line string
		pri  string
	}{
		{`{"level":"debug","msg":"x"}`, "<15>"},
		{`{"level":"info","msg":"x"}`, "<14>"},
		{`{"level":"warn","msg":"x"}`, "<12>"},
		{`{"level":"error","msg":"x"}`, "<11>"},
		{`{"level":"dpanic","msg":"x"}`, "<10>"},
		{`{"level":"panic","msg":"x"}`, "<9>"},
		{`{"level":"fatal","msg":"x"}`, "<8>"},
		{`{"level":"bogus","msg":"x"}`, "<14>"},
		{`{"msg":"no level"}`, "<14>"},
		{`plain text error`, "<14>"},
	}

	for _, tt := range tests {
		_, err := s.Write([]byte(tt.line))
		require.NoError(t, err)

		msg := readPacket(t, srv)
		assert.Contains(t, msg, tt.pri+"1 ", tt.line)
		assert.Contains(t, msg, tt.line)
	}
}

func TestSink_DefaultSeverity(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr, syslog.WithDefaultSeverity(syslog.SeverityNotice))
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("plain"))
	require.NoError(t, err)

	// user (1) * 8 + notice (5) = 13
	assert.Contains(t, readPacket(t, srv), "<13>1 ")
}

func TestSink_EmptyWriteSendsNothing(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr)
	require.NoError(t, err)
	defer s.Close()

	n, err := s.Write([]byte("\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	require.NoError(t, srv.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, _, err = srv.ReadFrom(make([]byte, 16))
	assert.Error(t, err)
}

func TestSink_HeaderFieldsSanitized(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr,
		syslog.WithAppName("my app"),
		syslog.WithHostname("héllo"),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("x"))
	require.NoError(t, err)

	assert.Regexp(t, ` hllo myapp \d+ - - x$`, readPacket(t, srv))
}

func TestSink_TCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	lines, _ := acceptLines(l)

	s, err := syslog.NewSink(l.Addr().String(),
		syslog.WithNetwork(syslog.NetworkTCP),
		syslog.WithAppName("myapp"),
		syslog.WithHostname("myhost"),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte(`{"level":"error","msg":"one"}` + "\n"))
	require.NoError(t, err)
	_, err = s.Write([]byte(`{"level":"info","msg":"two"}` + "\n"))
	require.NoError(t, err)

	assert.Regexp(t, `^<11>1 \S+ myhost myapp \d+ - - \{"level":"error","msg":"one"\}$`, nextLine(t, lines))
	assert.Regexp(t, `^<14>1 .* \{"level":"info","msg":"two"\}$`, nextLine(t, lines))
}

func TestSink_TCPEmbeddedNewlineStaysInOneMessage(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	lines, _ := acceptLines(l)

	s, err := syslog.NewSink(l.Addr().String(),
		syslog.WithNetwork(syslog.NetworkTCP),
		syslog.WithHostname("myhost"),
	)
	require.NoError(t, err)
	defer s.Close()

	forged := "<10>1 2026-01-01T00:00:00Z fakehost sshd - - - forged"
	_, err = s.Write([]byte("first\n" + forged + "\n"))
	require.NoError(t, err)
	_, err = s.Write([]byte("second\n"))
	require.NoError(t, err)

	assert.Regexp(t, `^<14>1 \S+ myhost \S+ \d+ - - first\n`+regexp.QuoteMeta(forged)+`$`, nextLine(t, lines))
	assert.Regexp(t, ` - - second$`, nextLine(t, lines))
}

func TestSink_TCPReconnects(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()

	lines, drop := acceptLines(l)

	s, err := syslog.NewSink(addr, syslog.WithNetwork(syslog.NetworkTCP))
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("before"))
	require.NoError(t, err)
	assert.Contains(t, nextLine(t, lines), "before")

	// Restart the server on the same address, dropping the open connection.
	require.NoError(t, l.Close())
	drop()
	l, err = net.Listen("tcp", addr)
	require.NoError(t, err)
	defer l.Close()
	lines, _ = acceptLines(l)

	// The first write after the restart may be lost in the dead connection's
	// buffer, so retry until the sink has reconnected.
	require.Eventually(t, func() bool {
		_, err := s.Write([]byte("after"))
		if err != nil {
			return false
		}
		select {
		case <-lines:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}, 3*time.Second, 10*time.Millisecond)
}

func TestSink_TLS(t *testing.T) {
	cert, pool := selfSignedCert(t)

	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	defer l.Close()
	lines, _ := acceptLines(l)

	s, err := syslog.NewSink(l.Addr().String(),
		syslog.WithTLSConfig(&tls.Config{RootCAs: pool}),
		syslog.WithAppName("myapp"),
		syslog.WithHostname("myhost"),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("secure"))
	require.NoError(t, err)

	assert.Regexp(t, `^<14>1 \S+ myhost myapp \d+ - - secure$`, nextLine(t, lines))
}

func TestSink_TLSUntrustedCertFails(t *testing.T) {
	cert, _ := selfSignedCert(t)

	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err == nil {
			c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()

	_, err = syslog.NewSink(l.Addr().String(), syslog.WithNetwork(syslog.NetworkTLS))
	assert.ErrorContains(t, err, "syslog: connect:")
}

func TestNewSink_ConnectError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	_, err = syslog.NewSink(addr, syslog.WithNetwork(syslog.NetworkTCP))
	assert.ErrorContains(t, err, "syslog: connect:")
}

func TestNewSink_UnsupportedNetwork(t *testing.T) {
	_, err := syslog.NewSink("127.0.0.1:514", syslog.WithNetwork("sctp"))
	assert.ErrorContains(t, err, `unsupported network "sctp"`)
}

func TestSink_WriteAfterClose(t *testing.T) {
	_, addr := listenUDP(t)

	s, err := syslog.NewSink(addr)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	_, err = s.Write([]byte("late"))
	assert.ErrorContains(t, err, "syslog: send log:")
	assert.NoError(t, s.Sync())
	assert.NoError(t, s.Close())
}

func TestSink_ErrorHandler(t *testing.T) {
	_, addr := listenUDP(t)

	var gotErr error
	s, err := syslog.NewSink(addr, syslog.WithErrorHandler(func(err error) { gotErr = err }))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	n, err := s.Write([]byte("late"))
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.ErrorContains(t, gotErr, "syslog: send log:")
}

func TestWithSyslog(t *testing.T) {
	srv, addr := listenUDP(t)

	opt, err := syslog.WithSyslog(addr, syslog.WithAppName("myapp"))
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Warn("from zap")

	// user (1) * 8 + warning (4) = 12
	msg := readPacket(t, srv)
	assert.Contains(t, msg, "<12>1 ")
	assert.Contains(t, msg, `"msg":"from zap"`)
}

func selfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},

		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(parsed)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

func TestSink_TCPServerGone(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, drop := acceptLines(l)

	s, err := syslog.NewSink(l.Addr().String(),
		syslog.WithNetwork(syslog.NetworkTCP),
		syslog.WithDialTimeout(time.Second),
	)
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, l.Close())
	drop()

	// Writes may still land in the dead connection's buffer; once the loss is
	// detected, reconnecting fails and the error is returned.
	require.Eventually(t, func() bool {
		_, err := s.Write([]byte("lost"))
		return err != nil
	}, 3*time.Second, 10*time.Millisecond)

	_, err = s.Write([]byte("still down"))
	assert.ErrorContains(t, err, "syslog: send log:")
}

func TestSink_EmptyHeaderFieldsBecomeNil(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr,
		syslog.WithAppName("   "),
		syslog.WithHostname("ééé"),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte("x"))
	require.NoError(t, err)

	// RFC 5424 NILVALUE "-" for fields with no printable characters.
	assert.Regexp(t, `^<14>1 \S+ - - \d+ - - x$`, readPacket(t, srv))
}

func TestSink_UnterminatedLevelUsesDefaultSeverity(t *testing.T) {
	srv, addr := listenUDP(t)

	s, err := syslog.NewSink(addr, syslog.WithDefaultSeverity(syslog.SeverityNotice))
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte(`{"level":"err`))
	require.NoError(t, err)

	// user (1) * 8 + notice (5) = 13
	assert.Contains(t, readPacket(t, srv), "<13>1 ")
}

func TestWithSyslog_Error(t *testing.T) {
	_, err := syslog.WithSyslog("127.0.0.1:514", syslog.WithNetwork("sctp"))
	assert.ErrorContains(t, err, `syslog: unsupported network "sctp"`)
}

func TestSink_Formats(t *testing.T) {
	tests := map[string]struct {
		format syslog.Format
		want   string
	}{
		"rfc5424": {
			format: syslog.FormatRFC5424,
			want:   `^<134>1 \d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}(Z|[+-]\d\d:\d\d) myhost myapp 42 - - hello$`,
		},
		"forward": {
			format: syslog.FormatForward,
			want:   `^<134>\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}(Z|[+-]\d\d:\d\d) myhost myapp\[42\]: hello$`,
		},
		"syslog protocol 23": {
			format: syslog.FormatSyslogProtocol23,
			want:   `^<134>1 \d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}(Z|[+-]\d\d:\d\d) myhost myapp 42 - - hello$`,
		},
		"traditional forward": {
			format: syslog.FormatTraditionalForward,
			want:   `^<134>[A-Z][a-z]{2} [ \d]\d \d\d:\d\d:\d\d myhost myapp\[42\]: hello$`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv, addr := listenUDP(t)

			s, err := syslog.NewSink(addr,
				syslog.WithFormat(tt.format),
				syslog.WithAppName("myapp"),
				syslog.WithHostname("myhost"),
				syslog.WithProcID("42"),
				syslog.WithFacility(syslog.FacilityLocal0),
			)
			require.NoError(t, err)
			defer s.Close()

			_, err = s.Write([]byte("hello\n"))
			require.NoError(t, err)

			assert.Regexp(t, tt.want, readPacket(t, srv))
		})
	}
}

func TestSink_TraditionalForwardOverTCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	lines, _ := acceptLines(l)

	s, err := syslog.NewSink(l.Addr().String(),
		syslog.WithNetwork(syslog.NetworkTCP),
		syslog.WithFormat(syslog.FormatTraditionalForward),
		syslog.WithHostname("myhost"),
		syslog.WithAppName("myapp"),
		syslog.WithProcID("42"),
	)
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Write([]byte(`{"level":"error","msg":"boom"}`))
	require.NoError(t, err)

	// user (1) * 8 + error (3) = 11
	assert.Regexp(t, `^<11>\S+ +\d+ \S+ myhost myapp\[42\]: \{"level":"error","msg":"boom"\}$`, nextLine(t, lines))
}

func TestSink_BSDTagLimitedTo32Chars(t *testing.T) {
	tests := map[string]struct {
		app, procID string
		want        string
	}{
		"fits":         {app: "myapp", procID: "42", want: "myapp[42]:"},
		"long app":     {app: strings.Repeat("a", 40), procID: "12345", want: strings.Repeat("a", 24) + "[12345]:"},
		"long proc id": {app: "myapp", procID: strings.Repeat("9", 40), want: "myapp:"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv, addr := listenUDP(t)

			s, err := syslog.NewSink(addr,
				syslog.WithFormat(syslog.FormatForward),
				syslog.WithHostname("h"),
				syslog.WithAppName(tt.app),
				syslog.WithProcID(tt.procID),
			)
			require.NoError(t, err)
			defer s.Close()

			_, err = s.Write([]byte("x"))
			require.NoError(t, err)

			msg := readPacket(t, srv)
			assert.True(t, strings.HasSuffix(msg, " h "+tt.want+" x"), msg)
			assert.LessOrEqual(t, len(tt.want), 32)
		})
	}
}

func TestNewSink_UnsupportedFormat(t *testing.T) {
	_, addr := listenUDP(t)

	_, err := syslog.NewSink(addr, syslog.WithFormat(syslog.Format(99)))
	assert.EqualError(t, err, "syslog: unsupported format 99")
}
