package papertrail_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/logger"

	"github.com/zauberhaus/logger/pkg/papertrail"
)

// newTestServer creates a test server whose health-check POST (empty body) returns 200
// and hands all other requests to handler.
func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := papertrail.NewSink(srv.URL, "bad-token",
		papertrail.WithHTTPClient(srv.Client()),
	)
	assert.ErrorContains(t, err, "papertrail: health check:")
}

func TestSink_RejectsHTTPByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent over plain http")
	}))
	defer srv.Close()

	_, err := papertrail.NewSink(srv.URL, "token")
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
}

func TestSink_AllowHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token", papertrail.AllowHTTP())
	require.NoError(t, err)
	require.NoError(t, writer.Close())
}

func TestSink_WriteDoesNotBlockOnSlowServer(t *testing.T) {
	release := make(chan struct{})
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(1),
	)
	require.NoError(t, err)

	start := time.Now()
	for i := range 50 {
		_, err := writer.Write(fmt.Appendf(nil, "line %d\n", i))
		require.NoError(t, err)
	}
	assert.Less(t, time.Since(start), 500*time.Millisecond)

	close(release)
	require.NoError(t, writer.Close())
}

func TestSink_QueueFullDropsLines(t *testing.T) {
	release := make(chan struct{})
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	defer close(release)

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(1),
		papertrail.WithQueueSize(2),
	)
	require.NoError(t, err)

	// The worker takes at most one line and blocks on the server; the queue
	// holds two more. Anything beyond that must be dropped.
	var dropped int
	for i := range 10 {
		if _, err := writer.Write(fmt.Appendf(nil, "line %d\n", i)); err != nil {
			assert.ErrorIs(t, err, papertrail.ErrQueueFull)
			dropped++
		}
	}
	assert.GreaterOrEqual(t, dropped, 7)
}

func TestSink_QueueFullReportedToErrorHandler(t *testing.T) {
	release := make(chan struct{})
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()
	defer close(release)

	var (
		mu   sync.Mutex
		errs []error
	)
	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(1),
		papertrail.WithQueueSize(1),
		papertrail.WithErrorHandler(func(err error) {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}),
	)
	require.NoError(t, err)

	for i := range 10 {
		_, err := writer.Write(fmt.Appendf(nil, "line %d\n", i))
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, errs)
	for _, err := range errs {
		assert.ErrorIs(t, err, papertrail.ErrQueueFull)
	}
}

func TestSink_CloseDeliversQueuedLines(t *testing.T) {
	var (
		mu    sync.Mutex
		lines []string
	)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		lines = append(lines, strings.Split(string(body), "\n")...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(7),
		papertrail.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 25 {
				_, err := writer.Write(fmt.Appendf(nil, "g%d line %d\n", g, i))
				assert.NoError(t, err)
			}
		})
	}
	wg.Wait()

	require.NoError(t, writer.Close())

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, lines, 100)
}

func TestSink_WriteAfterClose(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, writer.Close())

	_, err = writer.Write([]byte("late\n"))
	assert.ErrorIs(t, err, papertrail.ErrClosed)
	assert.NoError(t, writer.Sync())
}

func TestSink_FlushesOnInterval(t *testing.T) {
	got := make(chan string, 1)
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithFlushInterval(20*time.Millisecond),
	)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("tick\n"))
	require.NoError(t, err)

	select {
	case body := <-got:
		assert.Equal(t, "tick", body)
	case <-time.After(2 * time.Second):
		require.Fail(t, "line was not flushed on interval")
	}
}

func TestSink_BackgroundErrorReportedToHandler(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	errs := make(chan error, 1)
	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithBatchSize(1),
		papertrail.WithFlushInterval(time.Hour),
		papertrail.WithErrorHandler(func(err error) {
			select {
			case errs <- err:
			default:
			}
		}),
	)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("fails\n"))
	require.NoError(t, err)

	select {
	case err := <-errs:
		assert.ErrorContains(t, err, "papertrail: HTTP 500")
	case <-time.After(2 * time.Second):
		require.Fail(t, "background delivery error was not reported")
	}
}

func TestSink_SyncAndCloseReturnErrorsWithoutHandler(t *testing.T) {
	srv := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	writer, err := papertrail.NewSink(srv.URL, "token",
		papertrail.WithHTTPClient(srv.Client()),
		papertrail.WithFlushInterval(time.Hour),
	)
	require.NoError(t, err)

	_, err = writer.Write([]byte("one\n"))
	require.NoError(t, err)
	assert.ErrorContains(t, writer.Sync(), "papertrail: HTTP 500")

	_, err = writer.Write([]byte("two\n"))
	require.NoError(t, err)
	assert.ErrorContains(t, writer.Close(), "papertrail: HTTP 500")
}
