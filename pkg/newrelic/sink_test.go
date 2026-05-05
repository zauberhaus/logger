package newrelic_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/newrelic"
)

func TestSink_SendsBatchPayload(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotKey  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotKey = r.Header.Get("Api-Key")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w, err := newrelic.NewSink("test-license-key",
		newrelic.WithURL(srv.URL),
		newrelic.WithService("mysvc"),
		newrelic.WithHost("myhost"),
		newrelic.WithAttribute("env", "test"),
		newrelic.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)

	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"hello"}`))
	require.NoError(t, err)
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, "test-license-key", gotKey)

	// Payload: [{"common":{"attributes":{...}},"logs":[...]}]
	var batch []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &batch))
	require.Len(t, batch, 1)

	common := batch[0]["common"].(map[string]any)
	attrs := common["attributes"].(map[string]any)
	assert.Equal(t, "mysvc", attrs["service.name"])
	assert.Equal(t, "myhost", attrs["hostname"])
	assert.Equal(t, "test", attrs["env"])

	logs := batch[0]["logs"].([]any)
	require.Len(t, logs, 1)
	entry := logs[0].(map[string]any)
	assert.Equal(t, "info", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
}

func TestSink_PlainTextWrappedAsMessage(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)

	defer w.Close()

	w.Write([]byte("plain log line\n"))
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	var batch []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &batch))
	logs := batch[0]["logs"].([]any)
	entry := logs[0].(map[string]any)
	assert.Equal(t, "plain log line", entry["message"])
}

func TestSink_EURegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithRegion("EU"),
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer w.Close()
	assert.NotNil(t, w)
}

func TestSink_BatchesByCount(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
		newrelic.WithBatchSize(2),
		newrelic.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)
	defer w.Close()

	// start counting after construction so the health check is not included
	mu.Lock()
	callCount = 0
	mu.Unlock()
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})

	w.Write([]byte(`{"msg":"a"}`))
	w.Write([]byte(`{"msg":"b"}`))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()
	assert.Equal(t, 1, count)
}

func TestSink_ErrorHandler(t *testing.T) {
	var gotErr error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
		newrelic.WithErrorHandler(func(err error) { gotErr = err }),
	)
	require.NoError(t, err)
	defer w.Close()

	srv.Close() // health check passed; close so subsequent sends fail

	w.Write([]byte(`{"msg":"x"}`))
	w.Sync()

	assert.ErrorContains(t, gotErr, "newrelic: send logs:")
}

func TestSink_InvalidAPIKey(t *testing.T) {
	_, err := newrelic.NewSink("invalid")
	assert.ErrorContains(t, err, "403")
}
