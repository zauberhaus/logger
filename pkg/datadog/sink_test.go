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
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
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

func TestNewSink_KeyNotValid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"valid":false}`))
	}))
	defer srv.Close()

	_, err := datadog.NewSink("key", datadog.WithHTTPClient(redirectClient(srv.URL)))
	assert.EqualError(t, err, "datadog: invalid API key")
}

func TestNewSink_HealthCheckRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := datadog.NewSink("key", datadog.WithHTTPClient(redirectClient(srv.URL)))
	assert.ErrorContains(t, err, "datadog: health check:")
}

func TestSink_EmptyLineSkipped(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key", datadog.WithHTTPClient(redirectClient(srv.URL)))
	require.NoError(t, err)
	defer w.Close()

	n, err := w.Write([]byte("\n"))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, w.Sync())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, calls)
}

func TestSink_FlushesOnInterval(t *testing.T) {
	got := make(chan []byte, 1)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- body
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithFlushInterval(20*time.Millisecond),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("tick\n"))
	require.NoError(t, err)

	select {
	case body := <-got:
		var logs []map[string]any
		require.NoError(t, json.Unmarshal(body, &logs))
		require.Len(t, logs, 1)
		assert.Equal(t, "tick", logs[0]["message"])
	case <-time.After(2 * time.Second):
		require.Fail(t, "log was not flushed on interval")
	}
}

func TestSink_UnexpectedStatus(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithBatchSize(1),
	)
	require.NoError(t, err)
	defer w.Close()

	n, err := w.Write([]byte("x\n"))
	assert.EqualError(t, err, "datadog: HTTP 200")
	assert.Equal(t, 0, n)
}

func TestSink_JSONWithoutMsgAndSource(t *testing.T) {
	got := make(chan []byte, 1)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- body
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := datadog.NewSink("key",
		datadog.WithHTTPClient(redirectClient(srv.URL)),
		datadog.WithSource("go"),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte(`{"level":"info","count":3}` + "\n"))
	require.NoError(t, err)
	_, err = w.Write([]byte("plain\n"))
	require.NoError(t, err)
	require.NoError(t, w.Sync())

	var logs []map[string]any
	require.NoError(t, json.Unmarshal(<-got, &logs))
	require.Len(t, logs, 2)

	// Without a msg field the whole line becomes the message.
	assert.Equal(t, `{"level":"info","count":3}`, logs[0]["message"])
	assert.Equal(t, "info", logs[0]["level"])
	assert.Equal(t, "go", logs[0]["ddsource"])

	assert.Equal(t, "plain", logs[1]["message"])
	assert.Equal(t, "go", logs[1]["ddsource"])
}

func TestWithDatadog(t *testing.T) {
	got := make(chan []byte, 1)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- body
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	opt, err := datadog.WithDatadog("key", datadog.WithHTTPClient(redirectClient(srv.URL)))
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Info("from zap")
	require.NoError(t, log.Sync())

	select {
	case body := <-got:
		var logs []map[string]any
		require.NoError(t, json.Unmarshal(body, &logs))
		require.Len(t, logs, 1)
		assert.Equal(t, "from zap", logs[0]["message"])
	case <-time.After(2 * time.Second):
		require.Fail(t, "zap entry was not delivered")
	}
}

func TestWithDatadog_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := datadog.WithDatadog("key", datadog.WithHTTPClient(redirectClient(srv.URL)))
	assert.ErrorContains(t, err, "datadog: health check:")
}
