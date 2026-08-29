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
	drained  chan struct{}
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
		drained:  make(chan struct{}),
		logger:   logger,
	}

	go c.writePump()

	return c
}

func (c *connection) read(value any) error {
	return c.conn.ReadJSON(value)
}

// send reports whether the message is now certain to be written or dropped by
// the write pump. It drops the message once the client's queue is full, so a
// client that is not keeping up never blocks whoever is sending to it.
func (c *connection) send(value any) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	select {
	case c.outbound <- value:
	default:
		c.logger.Warn("outbound queue is full due to a slow connection, dropping the message", "remoteAddress", c.conn.RemoteAddr().String())

		return false
	}

	// the connection may have begun closing while the message was being queued,
	// in which case the pump can already have drained past it. Reporting a
	// failure here is the safe way round: the caller keeps the request
	// addressable instead of dropping a reply it believes was delivered.
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// writePump is the only goroutine that writes to the socket. It also sends the
// pings that keep the client's read deadline alive.
func (c *connection) writePump() {
	defer close(c.drained)

	ticker := time.NewTicker(c.config.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			c.drain()

			return

		case message := <-c.outbound:
			c.write(message, time.Now().Add(c.config.writeWait))

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.config.writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.logger.Warn("error on sending ping", "error", err)
			}
		}
	}
}

// drain writes out whatever the client was already queued to receive, so a
// message handed over just before the connection closed still reaches it. The
// grace period bounds it: a peer that has stopped reading cannot hold up the
// shutdown.
func (c *connection) drain() {
	deadline := time.Now().Add(c.config.closeGracePeriod)

	for time.Now().Before(deadline) {
		select {
		case message := <-c.outbound:
			if err := c.write(message, deadline); err != nil {
				return
			}
		default:
			return
		}
	}
}

func (c *connection) write(message any, deadline time.Time) error {
	_ = c.conn.SetWriteDeadline(deadline)

	err := c.conn.WriteJSON(message)
	if err != nil {
		c.logger.Warn("error on writing message to client", "error", err)
	}

	return err
}

// shutdown stops the write pump, lets it flush what is queued, and closes the
// socket. It is safe to call more than once.
func (c *connection) shutdown() error {
	c.close.Do(func() {
		close(c.done)
	})

	select {
	case <-c.drained:
	case <-time.After(c.config.closeGracePeriod):
	}

	// tell the client this was a normal close rather than a severed connection.
	_ = c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(c.config.writeWait),
	)

	return c.conn.Close()
}
