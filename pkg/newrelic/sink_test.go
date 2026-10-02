package newrelic_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/logger"

	"github.com/zauberhaus/logger/pkg/newrelic"
	zaplogger "github.com/zauberhaus/logger/pkg/zap"
)

func TestSink_SendsBatchPayload(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotKey  string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestSink_InvalidJSONWrappedAsMessage(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	injected := `{"a":1},{"message":"forged"}`
	w.Write([]byte(injected + "\n"))
	w.Sync()

	mu.Lock()
	defer mu.Unlock()

	var batch []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &batch))
	logs := batch[0]["logs"].([]any)
	require.Len(t, logs, 1)
	entry := logs[0].(map[string]any)
	assert.Equal(t, injected, entry["message"])
}

func TestSink_RejectsHTTPByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent over plain http")
	}))
	defer srv.Close()

	_, err := newrelic.NewSink("key", newrelic.WithURL(srv.URL))
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
}

func TestSink_AllowHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	w, err := newrelic.NewSink("key", newrelic.WithURL(srv.URL), newrelic.WithAllowHTTP())
	require.NoError(t, err)
	require.NoError(t, w.Close())
}

const healthCheckPayload = `[{"common":{},"logs":[{"message":"health check"}]}]`

// newLogAPIServer answers the NewSink health check with 202 and passes all
// other requests to handler.
func newLogAPIServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) == healthCheckPayload {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, r)
	}))
}

// hostRecorder records request hosts and answers every request with 202.
type hostRecorder struct {
	mu    sync.Mutex
	hosts []string
}

func (h *hostRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	h.mu.Lock()
	h.hosts = append(h.hosts, r.URL.Host)
	h.mu.Unlock()
	return &http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody, Header: http.Header{}}, nil
}

func TestSink_RegionEndpoints(t *testing.T) {
	tests := map[string]string{
		"EU": "log-api.eu.newrelic.com",
		"US": "log-api.newrelic.com",
		"":   "log-api.newrelic.com",
	}

	for region, host := range tests {
		t.Run(region, func(t *testing.T) {
			rec := &hostRecorder{}
			w, err := newrelic.NewSink("key",
				newrelic.WithRegion(region),
				newrelic.WithHTTPClient(&http.Client{Transport: rec}),
			)
			require.NoError(t, err)
			require.NoError(t, w.Close())

			rec.mu.Lock()
			defer rec.mu.Unlock()
			assert.Equal(t, []string{host}, rec.hosts)
		})
	}
}

func TestNewSink_HealthCheckRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := newrelic.NewSink("bad",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
	)
	assert.ErrorContains(t, err, "newrelic: health check: HTTP 403")
}

func TestNewSink_Unreachable(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := srv.Client()
	srv.Close()

	_, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(client),
	)
	assert.ErrorContains(t, err, "newrelic: health check:")
}

func TestSink_EmptyLineSkipped(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	srv := newLogAPIServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
	)
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
	srv := newLogAPIServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
		newrelic.WithFlushInterval(20*time.Millisecond),
	)
	require.NoError(t, err)
	defer w.Close()

	_, err = w.Write([]byte("tick\n"))
	require.NoError(t, err)

	select {
	case body := <-got:
		assert.Contains(t, body, `{"message":"tick"}`)
	case <-time.After(2 * time.Second):
		require.Fail(t, "log was not flushed on interval")
	}
}

func TestSink_WriteReturnsDeliveryErrorWithoutHandler(t *testing.T) {
	srv := newLogAPIServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	})
	defer srv.Close()

	w, err := newrelic.NewSink("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
		newrelic.WithBatchSize(1),
	)
	require.NoError(t, err)
	defer w.Close()

	n, err := w.Write([]byte("rejected\n"))
	assert.ErrorContains(t, err, "newrelic: HTTP 413")
	assert.Equal(t, 0, n)
}

func TestWithNewRelic(t *testing.T) {
	got := make(chan string, 1)
	srv := newLogAPIServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
		w.WriteHeader(http.StatusAccepted)
	})
	defer srv.Close()

	opt, err := newrelic.WithNewRelic("key",
		newrelic.WithURL(srv.URL),
		newrelic.WithHTTPClient(srv.Client()),
	)
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

func TestWithNewRelic_Error(t *testing.T) {
	_, err := newrelic.WithNewRelic("key", newrelic.WithURL("http://localhost:1"))
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
}
