package syslog_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
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

// acceptLines accepts connections on l and sends every received line to the
// returned channel. The returned function closes all accepted connections.
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
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					lines <- sc.Text()
				}
				_ = sc.Err()
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
