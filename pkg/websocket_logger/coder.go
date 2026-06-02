package websocket_logger

import (
	"context"
	"net/http"
	"time"

	ws "github.com/coder/websocket"
	"github.com/zauberhaus/logger/pkg/logger"
)

type coderLoggingDialer struct {
	url    string
	opts   *ws.DialOptions
	logger logger.Logger
}

func NewCoderLoggingDialer(url string, opts *ws.DialOptions, logger logger.Logger) CoderDialer {
	return &coderLoggingDialer{
		url:    url,
		opts:   opts,
		logger: logger,
	}
}

type coderLoggingConn struct {
	conn   *ws.Conn
	logger logger.Logger
}

func (c *coderLoggingConn) Write(ctx context.Context, messageType ws.MessageType, data []byte) error {
	err := c.conn.Write(ctx, messageType, data)
	if err != nil {
		c.logger.Errorf("[WS SEND FAILED] Type: %d | Error: %v", messageType, err)
	} else {
		c.logger.Debugf("[WS SEND] Type: %d | Data: %s", messageType, string(data))
	}
	return err
}

func (c *coderLoggingConn) Read(ctx context.Context) (ws.MessageType, []byte, error) {
	messageType, p, err := c.conn.Read(ctx)
	if err == nil {
		c.logger.Debugf("[WS RECV] Type: %d | Data: %s", messageType, string(p))
	} else {
		c.logger.Errorf("[WS RECV ERROR]: %v", err)
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

func (c *coderLoggingDialer) Dial(ctx context.Context) (CoderConnection, *http.Response, error) {
	if !c.logger.IsDebugEnabled() {
		return ws.Dial(ctx, c.url, c.opts)
	}

	c.logger.Debugf("[WS HANDSHAKE] Requesting: %s", c.url)
	start := time.Now()

	conn, resp, err := ws.Dial(ctx, c.url, c.opts)

	duration := time.Since(start)

	if err != nil {
		c.logger.Errorf("[WS HANDSHAKE FAILED] Error: %v | Duration: %v", err, duration)
		return nil, resp, err
	}

	status := ""
	if resp != nil {
		status = resp.Status
	}
	c.logger.Debugf("[WS HANDSHAKE SUCCESS] Status: %s | Duration: %v", status, duration)

	return &coderLoggingConn{
		conn:   conn,
		logger: c.logger,
	}, resp, nil
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

	status := ""
	if resp != nil {
		status = resp.Status
	}
	logger.Debugf("[WS HANDSHAKE SUCCESS] Status: %s | Duration: %v", status, duration)

	return &coderLoggingConn{
		conn:   conn,
		logger: logger,
	}, resp, nil
}
