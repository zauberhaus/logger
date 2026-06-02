package websocket_logger

import (
	"context"
	"net"
	"net/http"
	"time"

	coder "github.com/coder/websocket"
)

type CoderDialer interface {
	Dial(ctx context.Context, url string, header http.Header) (CoderConnection, *http.Response, error)
}

type GorillaDialer interface {
	Dial(ctx context.Context, url string, header http.Header) (GorillaConnection, *http.Response, error)
}

type CoderConnection interface {
	Write(ctx context.Context, messageType coder.MessageType, data []byte) error
	Read(ctx context.Context) (coder.MessageType, []byte, error)
	Close(code coder.StatusCode, reason string) error
	CloseNow() error
	Subprotocol() string
	Ping(ctx context.Context) error
}

type GorillaConnection interface {
	Subprotocol() string
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	WriteControl(messageType int, data []byte, deadline time.Time) error
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(t time.Time) error
	ReadMessage() (messageType int, p []byte, err error)
	SetReadDeadline(t time.Time) error
	SetReadLimit(limit int64)
	NetConn() net.Conn
	UnderlyingConn() net.Conn
	EnableWriteCompression(enable bool)
	SetCompressionLevel(level int) error
}
