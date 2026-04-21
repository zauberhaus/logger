package papertrail_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zauberhaus/logger/pkg/papertrail"
)

// newTestServer creates a test server whose health-check POST (empty body) returns 200
// and hands all other requests to handler.
func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
}

func TestSink_SendsLogLines(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotAuth string
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "test-token",
		papertrail.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("hello papertrail\n"))
	require.NoError(t, err)
	writer.Sync()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "Bearer test-token", gotAuth)
	assert.Equal(t, "hello papertrail", string(gotBody))
}

func TestSink_MultipleLinesBatched(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)
	defer writer.Close()

	writer.Write([]byte("line one\n"))
	writer.Write([]byte("line two\n"))
	writer.Sync()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "line one\nline two", string(gotBody))
}

func TestSink_EmptyLineSkipped(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	defer writer.Close()

	writer.Write([]byte("\n"))
	writer.Sync()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, callCount)
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
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(2),
		papertrail.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)
	defer writer.Close()

	writer.Write([]byte("a\n"))
	writer.Write([]byte("b\n"))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()
	assert.Equal(t, 1, count)
}

func TestSink_ErrorHandler(t *testing.T) {
	var (
		mu     sync.Mutex
		gotErr error
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithErrorHandler(func(err error) {
			mu.Lock()
			gotErr = err
			mu.Unlock()
		}),
	)
	require.NoError(t, err)
	defer writer.Close()

	writer.Write([]byte("oops\n"))
	writer.Sync()

	mu.Lock()
	defer mu.Unlock()
	assert.ErrorContains(t, gotErr, "papertrail: HTTP 500")
}

func TestSink_InvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := papertrail.NewSink(srv.URL, "bad-token",
		papertrail.WithHTTPClient(srv.Client()),
	)
	assert.ErrorContains(t, err, "papertrail: health check:")
}

func TestSink_InvalidAPIKey(t *testing.T) {
	_, err := papertrail.NewSink("https://logs.collector.solarwinds.com/v1/logs", "invalid")
	assert.ErrorContains(t, err, "401")
}
