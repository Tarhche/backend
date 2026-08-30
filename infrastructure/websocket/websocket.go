package websocket

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// Handler upgrades an HTTP request to a websocket and hands the client to the
// gateway. Everything protocol-specific lives here — the handshake, framing,
// JSON encoding, deadlines and keepalive — and nothing else does: what a
// request means, and where its reply comes from, is the gateway's business.
//
// A second transport is another type shaped like this one: give the gateway a
// gateway.Conn over TCP, a unix socket or a protocol of your own, and the rest
// of the machinery does not change.
type Handler struct {
	sessions gateway.Sessions
	upgrader websocket.Upgrader
	config   configuration
	logger   *slog.Logger
}

// make sure the handler implements the http.Handler interface
var _ http.Handler = &Handler{}

func NewHandler(sessions gateway.Sessions, logger *slog.Logger, options ...Option) (*Handler, error) {
	config, err := newConfiguration(options...)
	if err != nil {
		return nil, err
	}

	return &Handler{
		sessions: sessions,
		upgrader: websocket.Upgrader{CheckOrigin: config.checkOrigin},
		config:   config,
		logger:   logger,
	}, nil
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// a gateway that is shutting down can no longer carry a reply back, so
	// upgrading would give the client a connection whose answers never come.
	if err := h.sessions.Accept(); err != nil {
		h.logger.Warn("refusing a websocket connection", "error", err)
		http.Error(rw, "the service is shutting down", http.StatusServiceUnavailable)

		return
	}

	conn, err := h.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		h.logger.Error("failed to upgrade websocket connection", "error", err)

		return
	}

	h.logger.Info("new client connected", "remoteAddress", conn.RemoteAddr().String())

	h.sessions.Serve(r.Context(), newConnection(conn, h.config, h.logger))
}
