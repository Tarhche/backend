package task

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	attachtask "github.com/khanzadimahdi/testproject/application/runner/worker/task/attachTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

const (
	// writeWait bounds one write to the client, so a stalled peer cannot hold
	// the writer forever.
	writeWait = 10 * time.Second

	// readChunk is how much of a command's output is carried in one frame.
	readChunk = 4 << 10
)

// attachHandler carries a command running inside a container over a websocket.
//
// Binary frames are the command's own bytes, in both directions. A text frame
// is a control message, which today means a terminal that has been resized.
//
// The connection is only ever made by the runner manager, which is what decides
// who may open one; a worker is reachable inside the cluster and nowhere else.
type attachHandler struct {
	useCase  *attachtask.UseCase
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

var _ http.Handler = &attachHandler{}

func NewAttachHandler(useCase *attachtask.UseCase, logger *slog.Logger) *attachHandler {
	return &attachHandler{
		useCase: useCase,
		// the peer is the manager rather than a browser, so there is no origin
		// to check.
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		logger:   logger,
	}
}

// control is what a client sends to change something about a running command.
type control struct {
	Type string `json:"type"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

// @Summary		Attach to a worker task
// @Description	upgrades to a websocket carrying a command running inside the container
// @Tags			runner tasks
// @Param			uuid	path	string	true	"Task UUID"
// @Param			command	query	[]string	false	"The command to run; an interactive shell by default"
// @Success		101		{string}	string	"switching protocols"
// @Failure		404		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/attach [get]
func (h *attachHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &attachtask.Request{
		UUID:    r.PathValue("uuid"),
		Command: r.URL.Query()["command"],
		TTY:     true,
	}

	session, validationErrors, err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such container", http.StatusNotFound)

		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "could not attach to a container", "error", err)
		http.Error(rw, "could not attach", http.StatusInternalServerError)

		return
	case len(validationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(map[string]any{"errors": validationErrors})

		return
	}

	conn, err := h.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		_ = session.Close()
		h.logger.ErrorContext(r.Context(), "failed to upgrade an attach connection", "error", err)

		return
	}

	h.pump(conn, session)
}

// pump carries bytes between the client and the command until either end stops.
func (h *attachHandler) pump(conn *websocket.Conn, session container.ExecSession) {
	defer conn.Close()
	defer session.Close()

	done := make(chan struct{})

	// the command's output, on to the client.
	go func() {
		defer close(done)

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
					h.logger.Warn("a container's command ended", "error", err)
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
				h.logger.Warn("could not resize a terminal", "error", err)
			}
		}
	}
}
