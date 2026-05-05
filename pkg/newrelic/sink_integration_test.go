//go:build integration

package newrelic_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/newrelic"
)

func newrelicConfig(t *testing.T) string {
	t.Helper()

	key := os.Getenv("NEW_RELIC_LICENSE_KEY")
	if key == "" {
		t.Skip("NEW_RELIC_LICENSE_KEY not set")
	}

	return key
}

func TestSinkIntegration_SendsEvent(t *testing.T) {
	key := newrelicConfig(t)

	w, err := newrelic.NewSink(key,
		newrelic.WithService("integration-test"),
		newrelic.WithHost("testhost"),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"integration test"}`))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_PlainText(t *testing.T) {
	key := newrelicConfig(t)

	w, err := newrelic.NewSink(key)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_Batch(t *testing.T) {
	key := newrelicConfig(t)

	var gotErr error
	w, err := newrelic.NewSink(key,
		newrelic.WithBatchSize(3),
		newrelic.WithFlushInterval(time.Hour),
		newrelic.WithErrorHandler(func(err error) { gotErr = err }),
	)
	require.NoError(t, err)
	defer w.Close()

	for i := range 3 {
		_, err := w.Write([]byte(fmt.Sprintf(`{"msg":"event %d"}`, i)))
		require.NoError(t, err)
	}
	time.Sleep(200 * time.Millisecond)

	assert.NoError(t, gotErr)
}
