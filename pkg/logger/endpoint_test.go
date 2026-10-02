package logger_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger/pkg/logger"
)

func TestCheckEndpoint(t *testing.T) {
	tests := []struct {
		endpoint  string
		allowHTTP bool
		insecure  bool
		invalid   bool
	}{
		{endpoint: "https://example.com/logs"},
		{endpoint: "HTTPS://example.com/logs"},
		{endpoint: "http://example.com/logs", insecure: true},
		{endpoint: "http://example.com/logs", allowHTTP: true},
		{endpoint: "ftp://example.com/logs", invalid: true},
		{endpoint: "ftp://example.com/logs", allowHTTP: true, invalid: true},
		{endpoint: "example.com/logs", invalid: true},
		{endpoint: "https://", invalid: true},
		{endpoint: "://bad", invalid: true},
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			err := logger.CheckEndpoint(tt.endpoint, tt.allowHTTP)
			switch {
			case tt.insecure:
				assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
			case tt.invalid:
				assert.Error(t, err)
				assert.NotErrorIs(t, err, logger.ErrInsecureEndpoint)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckEndpoint_RedactsCredentials(t *testing.T) {
	err := logger.CheckEndpoint("http://user:secret@example.com/logs", false)
	require.ErrorIs(t, err, logger.ErrInsecureEndpoint)
	assert.NotContains(t, err.Error(), "secret")
}

func TestSecureClient_RejectsRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not reach the plain http server")
	}))
	defer plain.Close()

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusTemporaryRedirect)
	}))
	defer tlsSrv.Close()

	base := tlsSrv.Client()
	c := logger.SecureClient(base, false)

	_, err := c.Get(tlsSrv.URL)
	assert.ErrorIs(t, err, logger.ErrInsecureEndpoint)
	assert.Nil(t, base.CheckRedirect, "original client must not be modified")
}

func TestSecureClient_FollowsHTTPSRedirect(t *testing.T) {
	var target *httptest.Server
	target = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, target.URL+"/end", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	resp, err := logger.SecureClient(target.Client(), false).Get(target.URL + "/start")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestSecureClient_AllowHTTPReturnsClient(t *testing.T) {
	c := &http.Client{}
	assert.Same(t, c, logger.SecureClient(c, true))
	assert.Same(t, http.DefaultClient, logger.SecureClient(nil, true))
}

func TestSecureClient_KeepsCallerRedirectPolicy(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	base := srv.Client()
	base.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := logger.SecureClient(base, false).Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, resp.StatusCode)
}

func TestSecureClient_LimitsRedirects(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	_, err := logger.SecureClient(srv.Client(), false).Get(srv.URL + "/loop")
	assert.ErrorContains(t, err, "stopped after 10 redirects")
}
