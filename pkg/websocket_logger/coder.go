package websocket_logger

import (
	"context"
	"net/http"
	"time"

	ws "github.com/coder/websocket"
	"github.com/zauberhaus/logger/pkg/logger"
)

type coderLoggingConn struct {
	conn   *ws.Conn
	logger logger.Logger
}

func (c *coderLoggingConn) Write(ctx context.Context, messageType ws.MessageType, data []byte) error {
	c.logger.Debugf("[WS SEND] Type: %d | Data: %s", messageType, string(data))
	return c.conn.Write(ctx, messageType, data)
}

func (c *coderLoggingConn) Read(ctx context.Context) (ws.MessageType, []byte, error) {
	messageType, p, err := c.conn.Read(ctx)
	if err == nil {
		c.logger.Debugf("[WS RECV] Type: %d | Data: %s", messageType, string(p))
	} else {
		c.logger.Debugf("[WS RECV ERROR]: %v", err)
	}

	return messageType, p, err
}

func (c *coderLoggingConn) Close(code ws.StatusCode, reason string) error {
	start := time.Now()
	err := c.conn.Close(code, reason)
	duration := time.Since(start)

	if err != nil {
		c.logger.Errorf("[WS CLOSE FAILED] Error: %v (%v)", err, duration)
	} else {
		c.logger.Debugf("[WS CLOSE SUCCESS] %v: %s (%v)", code, reason, duration)
	}

	return err
}

func (c *coderLoggingConn) CloseNow() error {
	start := time.Now()
	err := c.conn.CloseNow()
	duration := time.Since(start)

	if err != nil {
		c.logger.Errorf("[WS CLOSE NOW FAILED] Error: %v (%v)", err, duration)
	} else {
		c.logger.Debugf("[WS CLOSE NOW SUCCESS] %v", duration)
	}

	return err
}

func (c *coderLoggingConn) Subprotocol() string {
	return c.conn.Subprotocol()
}

func (c *coderLoggingConn) Ping(ctx context.Context) error {
	start := time.Now()
	err := c.conn.Ping(ctx)
	duration := time.Since(start)

	if err != nil {
		c.logger.Errorf("[WS PING FAILED] Error: %v (%v)", err, duration)
	} else {
		c.logger.Debugf("[WS PING SUCCESS] %v", duration)
	}

	return err
}

func DialCoder(ctx context.Context, urlStr string, opts *ws.DialOptions, logger logger.Logger) (CoderConnection, *http.Response, error) {
	if !logger.IsDebugEnabled() {
		return ws.Dial(ctx, urlStr, opts)
	}

	logger.Debugf("[WS HANDSHAKE] Requesting: %s", urlStr)
	start := time.Now()

	conn, resp, err := ws.Dial(ctx, urlStr, opts)

	duration := time.Since(start)

	if err != nil {
		logger.Errorf("[WS HANDSHAKE FAILED] Error: %v | Duration: %v", err, duration)
		return nil, resp, err
	}

	logger.Debugf("[WS HANDSHAKE SUCCESS] Status: %s | Duration: %v", resp.Status, duration)

	return &coderLoggingConn{
		conn:   conn,
		logger: logger,
	}, resp, nil
}
