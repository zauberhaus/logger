//go:build integration

package papertrail_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/papertrail"
)

func papertrailConfig(t *testing.T) (endpoint, token string) {
	t.Helper()

	token = os.Getenv("PAPERTRAIL_TOKEN")
	if token == "" {
		t.Skip("PAPERTRAIL_TOKEN not set")
	}

	endpoint = os.Getenv("PAPERTRAIL_HOST")
	if endpoint == "" {
		t.Skip("PAPERTRAIL_HOST not set")
	}

	return endpoint, token
}

func TestSinkIntegration_SendsEvent(t *testing.T) {
	endpoint, token := papertrailConfig(t)

	w, err := papertrail.NewSink(endpoint, token)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"integration test"}`))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_PlainText(t *testing.T) {
	endpoint, token := papertrailConfig(t)

	w, err := papertrail.NewSink(endpoint, token)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_Batch(t *testing.T) {
	endpoint, token := papertrailConfig(t)

	var gotErr error
	w, err := papertrail.NewSink(endpoint, token,
		papertrail.WithBatchSize(3),
		papertrail.WithFlushInterval(time.Hour),
		papertrail.WithErrorHandler(func(err error) { gotErr = err }),
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
