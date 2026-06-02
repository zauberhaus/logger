//go:build integration

package splunk_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/splunk"
)

func hecConfig(t *testing.T) (hecURL string, clientOpt splunk.Option) {
	t.Helper()

	token := os.Getenv("SPLUNK_HEC_TOKEN")
	if token == "" {
		t.Skip("SPLUNK_HEC_TOKEN not set")
	}

	base := os.Getenv("SPLUNK_HEC_URL")
	if base == "" {
		base = "https://localhost:8088"
	}

	return base + "/services/collector/event", splunk.WithHTTPClient(&http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	})
}

func TestSinkIntegration_SendsEvent(t *testing.T) {
	hecURL, clientOpt := hecConfig(t)

	token := os.Getenv("SPLUNK_HEC_TOKEN")
	if token == "" {
		t.Skip("missing SPLUNK_HEC_TOKEN env variable")
	}

	w, err := splunk.NewSink(hecURL, token),
		clientOpt,
		splunk.WithSource("integration-test"),
		splunk.WithSourcetype("_json"),
		splunk.WithIndex("main"),
		splunk.WithHost("testhost"),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"integration test"}`))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_PlainText(t *testing.T) {
	hecURL, clientOpt := hecConfig(t)

	token := os.Getenv("SPLUNK_HEC_TOKEN")
	if token == "" {
		t.Skip("missing SPLUNK_HEC_TOKEN env variable")
	}

	w, err := splunk.NewSink(hecURL, token, clientOpt)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("plain text integration log\n"))
	require.NoError(t, err)

	assert.NoError(t, w.Sync())
}

func TestSinkIntegration_Batch(t *testing.T) {
	hecURL, clientOpt := hecConfig(t)

	token := os.Getenv("SPLUNK_HEC_TOKEN")
	if token == "" {
		t.Skip("missing SPLUNK_HEC_TOKEN env variable")
	}

	var gotErr error
	w, err := splunk.NewSink(hecURL, token,
		clientOpt,
		splunk.WithBatchSize(3),
		splunk.WithFlushInterval(time.Hour),
		splunk.WithErrorHandler(func(err error) { gotErr = err }),
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
