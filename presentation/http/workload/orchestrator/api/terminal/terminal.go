// Package terminal carries a command running on this node over a websocket,
// which is what a browser's terminal speaks.
//
// Binary frames are the command's own bytes, in both directions. A text frame
// is a control message, which today means a terminal that has been resized.
// Tasks and VMs speak it alike, so one terminal on the page opens either.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
)

const (
	// writeWait bounds one write to the client, so a stalled peer cannot hold
	// the writer forever.
	writeWait = 10 * time.Second

	// readChunk is how much of a command's output is carried in one frame.
	readChunk = 4 << 10
)

// Session is a command a terminal is attached to: reading takes its output,
// writing feeds its input, and closing lets go of it.
type Session interface {
	io.ReadWriteCloser

	// Resize tells the command's terminal how big it now is.
	Resize(ctx context.Context, rows uint, cols uint) error
}

// Upgrader opens a terminal's websocket.
//
// The origin is not what says who this is — the token is — and the peer may
// be a browser or anything else. Subprotocols is what accepts a browser's
// token: it offers "bearer" and the token itself, and a websocket is only
// opened if the server echoes one of them back. Echoing the marker rather
// than the token keeps the token out of the response.
func Upgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin:  func(*http.Request) bool { return true },
		Subprotocols: []string{middleware.WebSocketBearerProtocol},
	}
}

// control is what a client sends to change something about a running command.
type control struct {
	Type string `json:"type"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

// Pump carries bytes between the client and the command until either end
// stops, and closes both.
func Pump(conn *websocket.Conn, session Session, logger *slog.Logger) {
	defer conn.Close()
	defer session.Close()

	// the command's output, on to the client.
	go func() {
		// the command ending is the terminal ending: the client is let go,
		// which ends the loop below.
		defer conn.Close()

		buffer := make([]byte, readChunk)

		for {
			n, err := session.Read(buffer)

			if n > 0 {
				_ = conn.SetWriteDeadline(time.Now().Add(writeWait))

				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
					return
				}
			}

			if err != nil {
				if !errors.Is(err, io.EOF) {
					logger.Warn("a terminal's command ended", "error", err)
				}

				return
			}
		}
	}()

	// the client's input, on to the command. Closing the session is what
	// releases the reader above, so this loop ending ends both.
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if _, err := session.Write(payload); err != nil {
				return
			}

		case websocket.TextMessage:
			var message control
			if err := json.Unmarshal(payload, &message); err != nil {
				continue
			}

			if message.Type != "resize" || message.Rows == 0 || message.Cols == 0 {
				continue
			}

			if err := session.Resize(context.Background(), message.Rows, message.Cols); err != nil {
				logger.Warn("could not resize a terminal", "error", err)
			}
		}
	}
}
