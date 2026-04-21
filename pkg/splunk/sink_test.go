package splunk_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/splunk"
)

func TestSink_SendsHECEvent(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "test-token",
		splunk.WithSource("myapp"),
		splunk.WithSourcetype("_json"),
		splunk.WithIndex("main"),
		splunk.WithHost("myhost"),
		splunk.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"warn","msg":"watch out"}`))
	require.NoError(t, err)
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, "Splunk test-token", gotAuth)

	lines := strings.Split(strings.TrimSpace(string(gotBody)), "\n")
	require.Len(t, lines, 1)

	var event map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &event))

	ev, ok := event["event"].(map[string]any)
	require.True(t, ok, "event field should be a JSON object")
	assert.Equal(t, "warn", ev["level"])
	assert.Equal(t, "watch out", ev["msg"])
	assert.Equal(t, "myapp", event["source"])
	assert.Equal(t, "_json", event["sourcetype"])
	assert.Equal(t, "main", event["index"])
	assert.Equal(t, "myhost", event["host"])
}

func TestSink_PlainTextEvent(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer w.Close()

	w.Write([]byte("plain text log\n"))
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	lines := strings.Split(strings.TrimSpace(string(gotBody)), "\n")
	require.Len(t, lines, 1)

	var event map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &event))
	assert.Equal(t, "plain text log", event["event"])
	assert.Equal(t, "_json", event["sourcetype"])
}

func TestSink_MultipleEventsNDJSON(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithHTTPClient(srv.Client()),
		splunk.WithBatchSize(2),
		splunk.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)
	defer w.Close()

	w.Write([]byte(`{"msg":"first"}`))
	w.Write([]byte(`{"msg":"second"}`))
	// batchSize=2 triggers auto-flush
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	lines := strings.Split(strings.TrimSpace(string(gotBody)), "\n")
	assert.Len(t, lines, 2)
}

func TestSink_ErrorHandler(t *testing.T) {
	var gotErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithErrorHandler(func(err error) { gotErr = err }),
	)
	require.NoError(t, err)
	defer w.Close()

	srv.Close() // health check passed; close so subsequent sends fail

	w.Write([]byte(`{"msg":"x"}`))
	w.Sync()

	assert.ErrorContains(t, gotErr, "splunk: send logs:")
}
