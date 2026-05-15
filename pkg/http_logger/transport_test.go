// cspell:ignore andybalholm
package http_logger_test

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/zstd"
	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/http_logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/zap"
)

type MockErrorTransport struct{}

func (m *MockErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errors.New("network connection refused")
}

func TestLoggingTransportGet(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.NotContains(t, txt, "request body: ")
	assert.NotContains(t, txt, "response body: ")

}

func TestLoggingTransportWithBody(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))

	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body: "+string(body))
}

func TestLoggingTransportError(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 502 http://127.0.0.1:")
	assert.NotContains(t, txt, "request body: ")
	assert.NotContains(t, txt, "response body: ")
}

func TestLoggingTransportBrotliCompressed(t *testing.T) {

	payload := "request"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw := brotli.NewWriter(&b)
		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body: "+string(body))
	assert.True(t, resp.Header.Get("Content-Encoding") == "")
	assert.True(t, resp.Header.Get("Content-Length") == "7")
}

func TestLoggingTransportGzipCompressed(t *testing.T) {

	payload := "request"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw := gzip.NewWriter(&b)
		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "gz")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body: "+string(body))
	assert.True(t, resp.Header.Get("Content-Encoding") == "")
	assert.True(t, resp.Header.Get("Content-Length") == "7")
}

func TestLoggingTransportDeflateCompressed(t *testing.T) {

	payload := "request"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw, err := flate.NewWriter(&b, 5)
		require.NoError(t, err)

		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "deflate")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body: "+string(body))
	assert.True(t, resp.Header.Get("Content-Encoding") == "")
	assert.True(t, resp.Header.Get("Content-Length") == "7")
}

func TestLoggingTransportZstdCompressed(t *testing.T) {

	payload := "request"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw := zstd.NewWriter(&b)

		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "zstd")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body: "+string(body))
	assert.True(t, resp.Header.Get("Content-Encoding") == "")
	assert.True(t, resp.Header.Get("Content-Length") == "7")
}

func TestLoggingTransportUnknownEncoding(t *testing.T) {

	payload := "request"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw := lzw.NewWriter(&b, lzw.LSB, 8)

		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "compress")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	input := "test"
	reader := bytes.NewReader([]byte(input))

	req, err := http.NewRequest("POST", server.URL, reader)
	require.NoError(t, err)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> POST http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "request body: "+input)
	assert.Contains(t, txt, "response body:\n"+hex.Dump(body))
	assert.True(t, resp.Header.Get("Content-Encoding") == "compress")
	assert.True(t, resp.Header.Get("Content-Length") == "11")
}

func TestLoggingTransportNetworkError(t *testing.T) {

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(&MockErrorTransport{}, l),
		Timeout:   time.Second * 10,
	}

	_, err := client.Get("http://localhost:9999/test")
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://localhost:9999/test")
	assert.Contains(t, txt, "<-- ERROR GET http://localhost:9999/test")
	assert.Contains(t, txt, "network connection refused")
}

func TestLoggingTransportDebugDisabled(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.InfoLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "response", string(body))

	txt := string(l.Bytes())
	assert.NotContains(t, txt, "-->")
	assert.NotContains(t, txt, "<--")
}

func TestLoggingTransportGzipEncoding(t *testing.T) {

	payload := "response"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		bw := gzip.NewWriter(&b)
		bw.Write([]byte(payload))
		bw.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(b.Bytes())
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	// DisableCompression prevents Go's transport from auto-decompressing gzip,
	// so our LoggingTransport gets to handle the Content-Encoding itself.
	transport := &http.Transport{DisableCompression: true}
	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(transport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, payload, string(body))

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "response body: "+payload)
	assert.Equal(t, "", resp.Header.Get("Content-Encoding"))
}

func TestLoggingTransportIdentityEncoding(t *testing.T) {

	payload := "response"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "identity")
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(payload))
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, payload, string(body))

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "response body: "+payload)
	assert.Equal(t, "identity", resp.Header.Get("Content-Encoding"))
}

func TestLoggingTransportEmptyResponseBody(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 204 http://127.0.0.1:")
	assert.NotContains(t, txt, "response body:")
}

func TestLoggingTransportBinaryResponseBody(t *testing.T) {

	payload := []byte{0xff, 0xfe, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(payload)
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	logger.SetLogger(l)

	client := &http.Client{
		Transport: http_logger.NewLoggingTransport(http.DefaultTransport, l),
		Timeout:   time.Second * 10,
	}

	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "--> GET http://127.0.0.1:")
	assert.Contains(t, txt, "<-- 200 http://127.0.0.1:")
	assert.Contains(t, txt, "response body:\n")
	assert.Contains(t, txt, "ff fe 00 01")
}
