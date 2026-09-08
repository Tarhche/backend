package task

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	gettasklogs "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTaskLogs"
)

const (
	// pollInterval is how often the store is asked for what a container has
	// written since the last look.
	pollInterval = 500 * time.Millisecond

	// pollLimit is how many lines one look returns, which also bounds how fast
	// a backlog is drained.
	pollLimit = 200

	// streamWriteWait bounds one write to the client.
	streamWriteWait = 10 * time.Second
)

// logsStreamHandler follows a container's output.
//
// The lines are read from where they are kept rather than from the node running
// the container, so a stream survives the container stopping, the node going
// away, and the manager being asked by a replica that never held it.
type logsStreamHandler struct {
	useCase  *gettasklogs.UseCase
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

var _ http.Handler = &logsStreamHandler{}

func NewLogsStreamHandler(useCase *gettasklogs.UseCase, logger *slog.Logger) *logsStreamHandler {
	return &logsStreamHandler{
		useCase: useCase,
		// the peer is the blog rather than a browser, so there is no origin to
		// check.
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		logger:   logger,
	}
}

// @Summary		Follow a task's logs
// @Description	upgrades to a websocket carrying the container's output as it is written
// @Tags			runner tasks
// @Param			uuid	path	string	true	"Task UUID"
// @Param			after	query	string	false	"Only lines written after this moment (RFC3339)"
// @Success		101		{string}	string	"switching protocols"
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/logs/stream [get]
func (h *logsStreamHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	after := parseAfter(r)

	conn, err := h.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to upgrade a log stream", "error", err)

		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// the client says nothing on this stream, so a read is only ever how it
	// tells us it has gone.
	go func() {
		defer cancel()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	h.follow(ctx, conn, uuid, after)
}

// follow sends whatever has been written since the last look, over and over,
// until the client goes away.
func (h *logsStreamHandler) follow(ctx context.Context, conn *websocket.Conn, uuid string, after time.Time) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		response, err := h.useCase.Execute(ctx, &gettasklogs.Request{
			UUID:  uuid,
			After: after,
			Limit: pollLimit,
		})
		if err != nil {
			if ctx.Err() == nil {
				h.logger.ErrorContext(ctx, "error on reading a container's logs", "error", err, "uuid", uuid)
			}

			return
		}

		for _, line := range response.Items {
			_ = conn.SetWriteDeadline(time.Now().Add(streamWriteWait))

			if err := conn.WriteJSON(line); err != nil {
				return
			}

			after = line.At
		}

		// a full page means there is a backlog, which is drained without
		// waiting on the ticker.
		if len(response.Items) == pollLimit {
			continue
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}
