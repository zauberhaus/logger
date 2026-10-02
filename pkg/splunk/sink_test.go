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
	"github.com/zauberhaus/logger/pkg/logger"

	"github.com/zauberhaus/logger/pkg/splunk"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

func TestSink_SendsHECEvent(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotAuth string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithHTTPClient(srv.Client()),
		splunk.WithErrorHandler(func(err error) { gotErr = err }),
	)
	require.NoError(t, err)
	defer w.Close()

	srv.Close() // health check passed; close so subsequent sends fail

	w.Write([]byte(`{"msg":"x"}`))
	w.Sync()

	assert.ErrorContains(t, gotErr, "splunk: send logs:")
}

func TestSink_InvalidJSONCannotInjectFields(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithIndex("main"),
		splunk.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer w.Close()

	injected := `{"a":1},"index":"other","x":{}`
	w.Write([]byte(injected + "\n"))
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	lines := strings.Split(strings.TrimSpace(string(gotBody)), "\n")
	require.Len(t, lines, 1)

	var event map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &event))
	assert.Equal(t, injected, event["event"])
	assert.Equal(t, "main", event["index"])
	assert.NotContains(t, event, "x")
}

func TestSink_RejectsHTTPByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent over plain http")
	}))
	defer srv.Close()

	_, err := splunk.NewSink(srv.URL, "tok")
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
}

func TestSink_AllowHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok", splunk.WithAllowHTTP())
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

// newHECServer answers the NewSink health check with 200 and passes all
// other requests to handler.
func newHECServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) == `{"event":"health check"}` {
			w.WriteHeader(http.StatusOK)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		handler(w, r)
	}))
}

func TestNewSink_HealthCheckRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := splunk.NewSink(srv.URL, "bad", splunk.WithHTTPClient(srv.Client()))
	assert.ErrorContains(t, err, "splunk: health check: HTTP 403")
}

func TestNewSink_Unreachable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	srv.Close()

	_, err := splunk.NewSink(srv.URL, "tok", splunk.WithHTTPClient(client))
	assert.ErrorContains(t, err, "splunk: health check:")
}

func TestSink_EmptyLineSkipped(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	srv := newHECServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok", splunk.WithHTTPClient(srv.Client()))
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
	got := make(chan string, 1)
	srv := newHECServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithHTTPClient(srv.Client()),
		splunk.WithFlushInterval(20*time.Millisecond),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("tick\n"))
	require.NoError(t, err)

	select {
	case body := <-got:
		assert.Contains(t, body, `"event":"tick"`)
	case <-time.After(2 * time.Second):
		require.Fail(t, "event was not flushed on interval")
	}
}

func TestSink_WriteReturnsDeliveryErrorWithoutHandler(t *testing.T) {
	srv := newHECServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	defer srv.Close()

	w, err := splunk.NewSink(srv.URL, "tok",
		splunk.WithHTTPClient(srv.Client()),
		splunk.WithBatchSize(1),
	)
	require.NoError(t, err)
	defer w.Close()

	n, err := w.Write([]byte("rejected\n"))
	assert.ErrorContains(t, err, "splunk: HTTP 400")
	assert.Equal(t, 0, n)
}

func TestWithSplunk(t *testing.T) {
	got := make(chan string, 1)
	srv := newHECServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	opt, err := splunk.WithSplunk(srv.URL, "tok", splunk.WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	log := zaplogger.NewLogger(zaplogger.WithOutput(zaplogger.JSONOutput), opt)
	log.Info("from zap")
	require.NoError(t, log.Sync())

	select {
	case body := <-got:
		assert.Contains(t, body, `"msg":"from zap"`)
	case <-time.After(2 * time.Second):
		require.Fail(t, "zap entry was not delivered")
	}
}

func TestWithSplunk_Error(t *testing.T) {
	_, err := splunk.WithSplunk("http://localhost:8088", "tok")
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
}
