// cspell:ignore nosuchserver
package websocket_logger_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/websocket_logger"
	"github.com/zauberhaus/logger/pkg/zap"
)

func TestGorilla_Dialer_Dialer(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, resp, err := dialer.Dial(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://127.0.0.1:")
	assert.Contains(t, txt, "[WS HANDSHAKE SUCCESS]")
}

func TestGorilla_Dialer_DialerDebugDisabled(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.InfoLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, resp, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.NotContains(t, txt, "[WS HANDSHAKE]")
}

func TestGorilla_Dialer_DialerError(t *testing.T) {
	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, "ws://localhost:19999/nosuchserver", nil, l)
	_, _, err := dialer.Dial(context.Background())
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://localhost:19999/nosuchserver")
	assert.Contains(t, txt, "[WS HANDSHAKE FAILED]")
}

func TestGorilla_DialerWriteMessage(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteMessage(ws.TextMessage, []byte("hello"))
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SEND] Type: 1 | Data: hello")
}

func TestGorilla_DialerReadMessage(t *testing.T) {
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

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	msgType, p, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, ws.TextMessage, msgType)
	assert.Equal(t, "world", string(p))

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV] Type: 1 | Data: world")
}

func TestGorilla_DialerReadMessageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.Close()
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)

	_, _, err = conn.ReadMessage()
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV ERROR]")
}

func TestGorilla_DialerClose(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)

	err = conn.Close()
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CLOSE SUCCESS]")
}

func TestGorilla_DialerWriteControl(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteControl(ws.PingMessage, []byte("ping"), time.Now().Add(time.Second))
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CONTROL] Type: 9 | Data: ping")
}

func TestGorilla_DialerSetWriteDeadline(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	err = conn.SetWriteDeadline(deadline)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET WRITE DEADLINE]")
}

func TestGorilla_DialerSetReadDeadline(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	err = conn.SetReadDeadline(deadline)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET READ DEADLINE]")
}

func TestGorilla_DialerEnableWriteCompression(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	conn.EnableWriteCompression(true)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS ENABLE WRITE COMPRESSION] true")
}

func TestGorilla_DialerSetCompressionLevel(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	err = conn.SetCompressionLevel(6)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET COMPRESSION LEVEL] 6")
}

func TestGorilla_DialerSubprotocol(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "", conn.Subprotocol())
}

func TestGorilla_DialerLocalAddr(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.LocalAddr())
}

func TestGorilla_DialerRemoteAddr(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.RemoteAddr())
}

func TestGorilla_DialerSetReadLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteMessage(ws.TextMessage, []byte("exceeds limit"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)

	conn.SetReadLimit(5)

	_, _, err = conn.ReadMessage()
	require.Error(t, err)
	assert.ErrorIs(t, err, ws.ErrReadLimit)
}

func TestGorilla_DialerNetConn(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.NetConn())
}

func TestGorilla_DialerUnderlyingConn(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, wsURL(server), nil, l)
	conn, _, err := dialer.Dial(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.UnderlyingConn())
}
