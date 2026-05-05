package datadog_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/datadog"
)

// redirectClient returns an *http.Client whose transport rewrites every
// request's host/scheme to point at srvURL. This lets tests intercept the
// Datadog SDK's outbound calls without touching the SDK's server configuration.
func redirectClient(srvURL string) *http.Client {
	target, _ := url.Parse(srvURL)
	return &http.Client{
		Transport: &redirectTransport{base: http.DefaultTransport, target: target},
	}
}

type redirectTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = t.target.Scheme
	cloned.URL.Host = t.target.Host
	return t.base.RoundTrip(cloned)
}

// newTestServer wraps handler so that GET /api/v1/validate always returns
// {"valid":true}, allowing the NewWriter health check to pass in tests.
func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/validate" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"valid":true}`))
			return
		}
		handler(w, r)
	}))
}

func TestSink_SendsJSONLogLine(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotKey  string
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotKey = r.Header.Get("DD-API-KEY")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("test-key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithService("mysvc"),
		datadog.WithSource("go"),
		datadog.WithHost("myhost"),
		datadog.WithTags("env:test"),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","msg":"hello"}`))
	require.NoError(t, err)
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, "test-key", gotKey)

	// SDK serialises []HTTPLogItem; AdditionalProperties are inlined.
	var logs []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &logs))
	require.Len(t, logs, 1)

	entry := logs[0]
	assert.Equal(t, "hello", entry["message"]) // msg extracted → message
	assert.Equal(t, "info", entry["level"])    // remaining zap field inlined
	assert.Equal(t, "mysvc", entry["service"])
	assert.Equal(t, "go", entry["ddsource"])
	assert.Equal(t, "myhost", entry["hostname"])
	assert.Equal(t, "env:test", entry["ddtags"])
}

func TestSink_PlainTextWrappedAsMessage(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
	)
	require.NoError(t, err)
	defer w.Close()

	w.Write([]byte("plain log line\n"))
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	var logs []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &logs))
	require.Len(t, logs, 1)
	assert.Equal(t, "plain log line", logs[0]["message"])
}

func TestSink_BatchesByCount(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithBatchSize(3),
		datadog.WithFlushInterval(time.Hour), // disable periodic flush
	)
	require.NoError(t, err)
	defer w.Close()

	for i := 0; i < 3; i++ {
		w.Write([]byte(`{"msg":"x"}`))
	}
	// batchSize reached → auto-flush
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()
	assert.Equal(t, 1, count)
}

func TestSink_ErrorHandler(t *testing.T) {
	var gotErr error
	// Health check passes; log submissions return 500 to trigger the error handler.
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithErrorHandler(func(err error) { gotErr = err }),
	)
	require.NoError(t, err)
	defer w.Close()

	w.Write([]byte(`{"msg":"x"}`))
	w.Sync()

	assert.ErrorContains(t, gotErr, "datadog: submit logs:")
}

func TestSink_InvalidAPIKey(t *testing.T) {
	// Health check against the real Datadog API should reject an invalid key.
	_, err := datadog.NewSink("invalid")
	assert.ErrorContains(t, err, "datadog:")
}
