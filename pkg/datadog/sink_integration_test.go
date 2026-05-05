//go:build integration

package datadog_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/datadog"
)

func datadogConfig(t *testing.T) string {
	t.Helper()

	key := os.Getenv("DATADOG_API_KEY")
	if key == "" {
		t.Skip("DATADOG_API_KEY not set")
	}

	return key
}

func TestSinkIntegration_SendsEvent(t *testing.T) {
	key := datadogConfig(t)

	w, err := datadog.NewSink(key,
		datadog.WithService("integration-test"),
		datadog.WithHost("testhost"),
		datadog.WithSource("go"),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"integration test"}`))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_PlainText(t *testing.T) {
	key := datadogConfig(t)

	w, err := datadog.NewSink(key)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_Batch(t *testing.T) {
	key := datadogConfig(t)

	var gotErr error
	w, err := datadog.NewSink(key,
		datadog.WithBatchSize(3),
		datadog.WithFlushInterval(time.Hour),
		datadog.WithErrorHandler(func(err error) { gotErr = err }),
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
