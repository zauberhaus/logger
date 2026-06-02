// cspell:ignore nosuchserver
package websocket_logger_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGorillaWriteControl(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteControl(ws.PingMessage, []byte("ping"), time.Now().Add(time.Second))
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CONTROL] Type: 9 | Data: ping")
}

func TestGorillaSetWriteDeadline(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	err = conn.SetWriteDeadline(deadline)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET WRITE DEADLINE]")
}

func TestGorillaSetReadDeadline(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	err = conn.SetReadDeadline(deadline)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET READ DEADLINE]")
}

func TestGorillaEnableWriteCompression(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	conn.EnableWriteCompression(true)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS ENABLE WRITE COMPRESSION] true")
}

func TestGorillaSetCompressionLevel(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.SetCompressionLevel(6)
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET COMPRESSION LEVEL] 6")
}

func TestGorillaSubprotocol(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "", conn.Subprotocol())
}

func TestGorillaLocalAddr(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.LocalAddr())
}

func TestGorillaRemoteAddr(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.RemoteAddr())
}

func TestGorillaSetReadLimit(t *testing.T) {
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

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)

	conn.SetReadLimit(5)

	_, _, err = conn.ReadMessage()
	require.Error(t, err)
	assert.ErrorIs(t, err, ws.ErrReadLimit)
}

func TestGorillaNetConn(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.NetConn())
}

func TestGorillaUnderlyingConn(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	assert.NotNil(t, conn.UnderlyingConn())
}

func TestGorillaReadMessageErrorLevel(t *testing.T) {
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

func TestGorillaWriteMessageError(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	conn.Close()

	err = conn.WriteMessage(ws.TextMessage, []byte("after close"))
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SEND FAILED]")
}

func TestGorillaCloseError(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	conn.Close()

	err = conn.Close()
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CLOSE FAILED]")
}

func TestGorillaWriteControlError(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	conn.Close()

	err = conn.WriteControl(ws.PingMessage, []byte("ping"), time.Now().Add(time.Second))
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CONTROL]")
}

func TestGorillaSetReadDeadlineError(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	conn.NetConn().Close()

	err = conn.SetReadDeadline(time.Now().Add(time.Second))
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET READ DEADLINE FAILED]")
}

func TestGorillaSetCompressionLevelError(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	conn, _, err := websocket_logger.DialGorilla(context.Background(), ws.DefaultDialer, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.SetCompressionLevel(99)
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SET COMPRESSION LEVEL FAILED]")
}

func TestNewGorillaLoggingDialerSuccess(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, l)
	conn, resp, err := dialer.Dial(context.Background(), wsURL(server), nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://127.0.0.1:")
	assert.Contains(t, txt, "[WS HANDSHAKE SUCCESS]")
}

func TestNewGorillaLoggingDialerDebugDisabled(t *testing.T) {
	server := newGorillaEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.InfoLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, l)
	conn, resp, err := dialer.Dial(context.Background(), wsURL(server), nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close()

	txt := string(l.Bytes())
	assert.NotContains(t, txt, "[WS HANDSHAKE]")
}

func TestNewGorillaLoggingDialerError(t *testing.T) {
	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))

	dialer := websocket_logger.NewGorillaLoggingDialer(ws.DefaultDialer, l)
	_, _, err := dialer.Dial(context.Background(), "ws://localhost:19999/nosuchserver", nil)
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://localhost:19999/nosuchserver")
	assert.Contains(t, txt, "[WS HANDSHAKE FAILED]")
}
