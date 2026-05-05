package websocket_logger

import (
	"context"
	"net"
	"time"

	ws "github.com/coder/websocket"
)

type CoderConnection interface {
	Write(ctx context.Context, messageType ws.MessageType, data []byte) error
	Read(ctx context.Context) (ws.MessageType, []byte, error)
	Close(code ws.StatusCode, reason string) error
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
