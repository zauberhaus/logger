//go:build integration

package syslog_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/syslog"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

const (
	defaultIntegrationNetwork = syslog.NetworkTCP
)

// integrationConfig returns the address and network of the syslog server.
// Defaults to TCP on taillight.kube.nz:514, overridable with SYSLOG_ADDR and
// SYSLOG_NETWORK (udp, tcp or tcp+tls).
func integrationConfig(t *testing.T) (addr, network string) {
	addr = os.Getenv("SYSLOG_ADDR")
	if addr == "" {
		t.Skip("SYSLOG_ADDR missing")
	}

	network = os.Getenv("SYSLOG_NETWORK")
	if network == "" {
		network = defaultIntegrationNetwork
	}

	return addr, network
}

func newIntegrationSink(t *testing.T, opts ...syslog.Option) logger.Sink {
	t.Helper()

	addr, network := integrationConfig(t)

	var gotErr error
	opts = append([]syslog.Option{
		syslog.WithNetwork(network),
		syslog.WithAppName("logger-integration-test"),
		syslog.WithErrorHandler(func(err error) { gotErr = err }),
	}, opts...)

	s, err := syslog.NewSink(addr, opts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, s.Close())
		assert.NoError(t, gotErr)
	})

	return s
}

func TestSinkIntegration_PlainText(t *testing.T) {
	s := newIntegrationSink(t)

	_, err := s.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, s.Sync())
}

func TestSinkIntegration_Severities(t *testing.T) {
	s := newIntegrationSink(t)

	for _, level := range []string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"} {
		_, err := s.Write([]byte(fmt.Sprintf(`{"level":%q,"msg":"integration severity %s"}`, level, level)))
		require.NoError(t, err)
	}

	assert.NoError(t, s.Sync())
}

func TestSinkIntegration_Zap(t *testing.T) {
	addr, network := integrationConfig(t)

	opt, err := syslog.WithSyslog(addr,
		syslog.WithNetwork(network),
		syslog.WithAppName("logger-integration-test"),
		syslog.WithFacility(syslog.FacilityLocal0),
	)
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Infof("zap integration test at %s", time.Now().Format(time.RFC3339))
	log.Warn("zap integration warning")
	log.Error("zap integration error")

	assert.NoError(t, log.Sync())
}

func TestSinkIntegration_WriteAfterClose(t *testing.T) {
	addr, network := integrationConfig(t)
	s, err := syslog.NewSink(addr,
		syslog.WithNetwork(network),
		syslog.WithAppName("logger-integration-test"),
	)
	require.NoError(t, err)

	_, err = s.Write([]byte("before close"))
	require.NoError(t, err)
	require.NoError(t, s.Close())

	_, err = s.Write([]byte("after close"))
	assert.Error(t, err)
}
