package websocket

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// connection is one client's socket, seen as the transport the gateway drives.
// Every reply goes through writePump, which keeps gorilla's requirement of a
// single writer. Pings are the one thing written from elsewhere, because
// WriteControl is safe alongside the pump and a ping that queues behind a slow
// reply would let a healthy client's read deadline lapse.
type connection struct {
	conn     *websocket.Conn
	config   configuration
	outbound chan *domain.Reply
	done     chan struct{}
	drained  chan struct{}
	stop     sync.Once
	shut     sync.Once
	shutErr  error
	logger   *slog.Logger
}

// make sure the connection implements the transport the gateway expects
var _ gateway.Conn = &connection{}

// newConnection prepares an upgraded socket and starts its write pump and
// keepalive. The caller must call Close to stop them.
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
		outbound: make(chan *domain.Reply, config.outboundBuffer),
		done:     make(chan struct{}),
		drained:  make(chan struct{}),
		logger:   logger,
	}

	go c.writePump()
	go c.pingLoop()

	return c
}

// Read decodes the next request from the client. A conversation that ended —
// the client hung up, or this side closed the socket — is reported as io.EOF,
// which is how the gateway tells an ordinary goodbye from a broken transport.
func (c *connection) Read(request *domain.Request) error {
	err := c.conn.ReadJSON(request)
	if err == nil {
		return nil
	}

	// a connection this side closed is the end of the conversation, whatever
	// the transport happens to report for reading on it afterwards.
	select {
	case <-c.done:
		return io.EOF
	default:
	}

	var closed *websocket.CloseError

	if errors.As(err, &closed) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		c.logger.Debug("client connection ended", "reason", err, "remoteAddress", c.conn.RemoteAddr().String())

		return io.EOF
	}

	return err
}

// Send reports whether the reply is now certain to be written or dropped by the
// write pump. It refuses the reply once the client's queue is full, so a client
// that is not keeping up never blocks whoever is sending to it.
func (c *connection) Send(reply *domain.Reply) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	select {
	case c.outbound <- reply:
	default:
		c.logger.Warn("outbound queue is full due to a slow connection, dropping the reply", "remoteAddress", c.conn.RemoteAddr().String())

		return false
	}

	// the connection may have begun closing while the reply was being queued,
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

// Close stops the write pump, lets it flush what is queued, and closes the
// socket. It is safe to call more than once, and from any goroutine: closing
// the socket is what releases a client parked in Read.
func (c *connection) Close() error {
	c.stop.Do(func() {
		close(c.done)
	})

	// the pump may be part-way through a write when it is told to stop, and
	// that write has until writeWait to finish. Waiting only for the grace
	// period would close the socket underneath it and truncate the very reply
	// the drain exists to deliver.
	select {
	case <-c.drained:
	case <-time.After(c.config.closeGracePeriod + c.config.writeWait):
		c.logger.Warn("the write pump did not finish in time, closing the connection anyway", "remoteAddress", c.conn.RemoteAddr().String())
	}

	// the socket is closed once however many callers there are, so a second
	// Close reports what the first one found rather than "already closed".
	c.shut.Do(func() {
		// tell the client this was a normal close rather than a severed
		// connection.
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(c.config.writeWait),
		)

		c.shutErr = c.conn.Close()
	})

	return c.shutErr
}

// writePump is the only goroutine that writes replies to the socket.
func (c *connection) writePump() {
	defer close(c.drained)

	for {
		// checked on its own first: with both cases ready Go picks at random,
		// and every reply taken that way is written on the full write wait
		// rather than within the close grace period.
		select {
		case <-c.done:
			c.drain()

			return
		default:
		}

		select {
		case <-c.done:
			c.drain()

			return

		case reply := <-c.outbound:
			_ = c.write(reply, time.Now().Add(c.config.writeWait))
		}
	}
}

// pingLoop keeps the client's read deadline alive. Control frames may be
// written concurrently with the pump, so a ping is never held up behind a reply
// that is taking its time.
func (c *connection) pingLoop() {
	ticker := time.NewTicker(c.config.pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return

		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.config.writeWait)); err != nil {
				c.logger.Warn("error on sending ping", "error", err)
			}
		}
	}
}

// drain writes out whatever the client was already queued to receive, so a
// reply handed over just before the connection closed still reaches it. The
// grace period bounds it: a peer that has stopped reading cannot hold up the
// shutdown.
func (c *connection) drain() {
	deadline := time.Now().Add(c.config.closeGracePeriod)

	for time.Now().Before(deadline) {
		select {
		case reply := <-c.outbound:
			if err := c.write(reply, deadline); err != nil {
				return
			}
		default:
			return
		}
	}
}

func (c *connection) write(reply *domain.Reply, deadline time.Time) error {
	_ = c.conn.SetWriteDeadline(deadline)

	err := c.conn.WriteJSON(reply)
	if err != nil {
		c.logger.Warn("error on writing a reply to the client", "error", err)
	}

	return err
}
