//go:build integration

package cloudwatch_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/cloudwatch"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

// cloudWatchConfig returns the log group to test against. Credentials and
// region come from the default AWS configuration chain (AWS_PROFILE,
// AWS_REGION, ...). The stream is unique per test run.
func cloudWatchConfig(t *testing.T) (group, stream string) {
	t.Helper()

	group = os.Getenv("CLOUDWATCH_LOG_GROUP")
	if group == "" {
		t.Skip("CLOUDWATCH_LOG_GROUP not set")
	}

	return group, fmt.Sprintf("logger-integration-%d", time.Now().UnixNano())
}

func TestSinkIntegration_SendsEvents(t *testing.T) {
	group, stream := cloudWatchConfig(t)

	s, err := cloudwatch.NewSink(group, stream, cloudwatch.WithCreateLogGroup())
	require.NoError(t, err)

	_, err = s.Write([]byte(`{"level":"info","msg":"integration test"}` + "\n"))
	require.NoError(t, err)
	_, err = s.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, s.Sync())
	assert.NoError(t, s.Close())
}

func TestSinkIntegration_Zap(t *testing.T) {
	group, stream := cloudWatchConfig(t)

	opt, err := cloudwatch.WithCloudWatch(group, stream)
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Infof("integration via zap at %s", time.Now().Format(time.RFC3339))
	log.Warn("integration warning")

	assert.NoError(t, log.Sync())
}
