package websocket_logger

import (
	"context"
	"net/http"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/zauberhaus/logger/pkg/logger"
)

type GorillaConnection interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

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
		c.logger.Debugf("[WS RECV] Type: %d | Data: %s", messageType, string(p))
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

// LoggingDialer wraps the gorilla dialer to log the handshake
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
