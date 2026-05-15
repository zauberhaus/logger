package websocket_logger

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/zauberhaus/logger/pkg/logger"
)

type gorillaLoggingConn struct {
	conn   *ws.Conn
	logger logger.Logger
}

func (c *gorillaLoggingConn) WriteMessage(messageType int, data []byte) error {
	c.logger.Debugf("[WS SEND] Type: %d | Data: %s", messageType, string(data))
	return c.conn.WriteMessage(messageType, data)
}

func (c *gorillaLoggingConn) ReadMessage() (messageType int, p []byte, err error) {
	messageType, p, err = c.conn.ReadMessage()
	if err == nil {
		c.logger.Debugf("[WS RECV] Type: %d | Data: %s", messageType, strings.Trim(string(p), "\t\r\n"))
	} else {
		c.logger.Debugf("[WS RECV ERROR]: %v", err)
	}
	return
}

func (c *gorillaLoggingConn) Close() error {
	start := time.Now()
	err := c.conn.Close()
	duration := time.Since(start)

	if err != nil {
		c.logger.Errorf("[WS CLOSE FAILED] Error: %v (%v)", err, duration)
	} else {
		c.logger.Debugf("[WS CLOSE SUCCESS] %v", duration)
	}

	return err
}

func (c *gorillaLoggingConn) Subprotocol() string {
	return c.conn.Subprotocol()
}

func (c *gorillaLoggingConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *gorillaLoggingConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *gorillaLoggingConn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.logger.Debugf("[WS CONTROL] Type: %d | Data: %s (deadline %v)", messageType, string(data), deadline)
	return c.conn.WriteControl(messageType, data, deadline)
}

func (c *gorillaLoggingConn) SetWriteDeadline(t time.Time) error {
	c.logger.Debugf("[WS SET WRITE DEADLINE] %v", t)
	return c.conn.SetWriteDeadline(t)
}

func (c *gorillaLoggingConn) SetReadDeadline(t time.Time) error {
	c.logger.Debugf("[WS SET READ DEADLINE] %v", t)
	return c.conn.SetReadDeadline(t)
}

func (c *gorillaLoggingConn) SetReadLimit(limit int64) {
	c.conn.SetReadLimit(limit)
}

func (c *gorillaLoggingConn) NetConn() net.Conn {
	return c.conn.NetConn()
}

func (c *gorillaLoggingConn) UnderlyingConn() net.Conn {
	return c.conn.UnderlyingConn()
}

func (c *gorillaLoggingConn) EnableWriteCompression(enable bool) {
	c.logger.Debugf("[WS ENABLE WRITE COMPRESSION] %v", enable)
	c.conn.EnableWriteCompression(enable)
}

func (c *gorillaLoggingConn) SetCompressionLevel(level int) error {
	c.logger.Debugf("[WS SET COMPRESSION LEVEL] %d", level)
	return c.conn.SetCompressionLevel(level)
}

func DialGorilla(ctx context.Context, dialer *ws.Dialer, urlStr string, requestHeader http.Header, logger logger.Logger) (GorillaConnection, *http.Response, error) {
	if !logger.IsDebugEnabled() {
		return dialer.DialContext(ctx, urlStr, requestHeader)
	}

	logger.Debugf("[WS HANDSHAKE] Requesting: %s", urlStr)
	start := time.Now()

	conn, resp, err := dialer.DialContext(ctx, urlStr, requestHeader)

	duration := time.Since(start)

	if err != nil {
		logger.Errorf("[WS HANDSHAKE FAILED] Error: %v | Duration: %v", err, duration)
		return nil, resp, err
	}

	logger.Debugf("[WS HANDSHAKE SUCCESS] Status: %s | Duration: %v", resp.Status, duration)

	return &gorillaLoggingConn{
		conn:   conn,
		logger: logger,
	}, resp, nil
}
