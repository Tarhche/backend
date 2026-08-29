package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// connection owns a single client socket. Every write goes through writePump,
// which keeps gorilla's requirement of one writer at a time.
type connection struct {
	conn     *websocket.Conn
	config   configuration
	outbound chan any
	done     chan struct{}
	close    sync.Once
	logger   *slog.Logger
}

// make sure the connection implements the conn interface
var _ conn = &connection{}

// newConnection prepares an upgraded socket and starts its write pump. The
// caller must call shutdown to stop it.
func newConnection(conn *websocket.Conn, config configuration, logger *slog.Logger) *connection {
	conn.SetReadLimit(config.maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(config.pongWait))

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(config.pongWait))
	})

	conn.SetCloseHandler(func(code int, text string) error {
		logger.Info("client disconnected", "code", code, "text", text, "remoteAddress", conn.RemoteAddr().String())

		return nil
	})

	c := &connection{
		conn:     conn,
		config:   config,
		outbound: make(chan any, config.outboundBuffer),
		done:     make(chan struct{}),
		logger:   logger,
	}

	go c.writePump()

	return c
}

func (c *connection) read(value any) error {
	return c.conn.ReadJSON(value)
}

// send drops the message once the client's queue is full, so a client that is
// not keeping up never blocks whoever is sending to it.
func (c *connection) send(value any) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	select {
	case c.outbound <- value:
		return true
	case <-c.done:
		return false
	default:
		c.logger.Warn("outbound queue is full due to a slow connection, dropping the message", "remoteAddress", c.conn.RemoteAddr().String())

		return false
	}
}

// writePump is the only goroutine that writes to the socket. It also sends the
// pings that keep the client's read deadline alive.
func (c *connection) writePump() {
	ticker := time.NewTicker(c.config.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return

		case message := <-c.outbound:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.config.writeWait))
			if err := c.conn.WriteJSON(message); err != nil {
				c.logger.Warn("error on writing message to client", "error", err)
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.config.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Warn("error on sending ping", "error", err)
			}
		}
	}
}

// shutdown stops the write pump and closes the socket. It is safe to call more
// than once.
func (c *connection) shutdown() error {
	c.close.Do(func() {
		close(c.done)
	})

	return c.conn.Close()
}
