package websocket_logger_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	ws "github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/websocket_logger"
	"github.com/zauberhaus/logger/pkg/zap"
)

func newCoderEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		for {
			msgType, msg, err := conn.Read(ctx)
			if err != nil {
				return
			}
			conn.Write(ctx, msgType, msg)
		}
	}))
}

func TestDialCoder(t *testing.T) {
	server := newCoderEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	conn, resp, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close(ws.StatusNormalClosure, "")

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://127.0.0.1:")
	assert.Contains(t, txt, "[WS HANDSHAKE SUCCESS]")
}

func TestDialCoderDebugDisabled(t *testing.T) {
	server := newCoderEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.InfoLevel))
	ctx := context.Background()

	conn, resp, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer conn.Close(ws.StatusNormalClosure, "")

	txt := string(l.Bytes())
	assert.NotContains(t, txt, "[WS HANDSHAKE]")
}

func TestDialCoderError(t *testing.T) {
	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	_, _, err := websocket_logger.DialCoder(ctx, "ws://localhost:19999/nosuchserver", nil, l)
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS HANDSHAKE] Requesting: ws://localhost:19999/nosuchserver")
	assert.Contains(t, txt, "[WS HANDSHAKE FAILED]")
}

func TestCoderWrite(t *testing.T) {
	server := newCoderEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	conn, _, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close(ws.StatusNormalClosure, "")

	err = conn.Write(ctx, ws.MessageText, []byte("hello"))
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS SEND] Type: 1 | Data: hello")
}

func TestCoderRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		conn.Write(ctx, ws.MessageText, []byte("world"))
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	conn, _, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)
	defer conn.Close(ws.StatusNormalClosure, "")

	msgType, p, err := conn.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, ws.MessageText, msgType)
	assert.Equal(t, "world", string(p))

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV] Type: 1 | Data: world")
}

func TestCoderReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, &ws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		conn.Close(ws.StatusGoingAway, "bye")
	}))
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	conn, _, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)

	_, _, err = conn.Read(ctx)
	require.Error(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS RECV ERROR]")
}

func TestCoderClose(t *testing.T) {
	server := newCoderEchoServer(t)
	defer server.Close()

	l := memory.NewLogger(zap.WithLevel(logger.DebugLevel))
	ctx := context.Background()

	conn, _, err := websocket_logger.DialCoder(ctx, wsURL(server), nil, l)
	require.NoError(t, err)

	err = conn.Close(ws.StatusNormalClosure, "done")
	require.NoError(t, err)

	txt := string(l.Bytes())
	assert.Contains(t, txt, "[WS CLOSE SUCCESS]")
	assert.Contains(t, txt, "done")
}
