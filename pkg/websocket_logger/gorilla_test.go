package websocket_logger_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/websocket_logger"
	"github.com/zauberhaus/logger/pkg/zap"
)

var upgrader = ws.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func newGorillaEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			conn.WriteMessage(msgType, msg)
		}
	}))
}

func TestDialGorilla(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, resp, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://127.0.0.1:")
	assert.Contains(t, txt, "[WS HANDSHAKE SUCCESS]")
}

func TestDialGorillaDebugDisabled(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.InfoLevel))

	conn, resp, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.NotContains(t, txt, "[WS HANDSHAKE]")
}

func TestDialGorillaError(t *testing.T) {
	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	_, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, "ws://localhost:19999/nosuchserver", nil, l)
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://localhost:19999/nosuchserver")
	assert.Contains(t, txt, "[WS HANDSHAKE FAILED]")
}

func TestGorillaWriteMessage(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteMessage(ws.TextMessage, []byte("hello"))
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SEND] Type: 1 | Data: hello")
}

func TestGorillaReadMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteMessage(ws.TextMessage, []byte("world"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	msgType, p, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.TextMessage, msgType)
	assert.Equal(t, "world", string(p))

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV] Type: 1 | Data: world")
}

func TestGorillaReadMessageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)

	_, _, err = conn.ReadMessage()
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV ERROR]")
}

func TestGorillaClose(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)

	err = conn.Close()
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CLOSE SUCCESS]")
}
