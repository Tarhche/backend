package task

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	attachtask "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/attachTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/terminal"
)

// endWait bounds ending a command the client walked away from, which is two
// signals with a grace period after each.
const endWait = 30 * time.Second

// attachHandler carries a command running inside a task over a websocket, as
// the terminal package speaks it.
//
// Who is asking comes from the token the middleware verified, and whose
// task it is comes off the task itself. Nothing in between is trusted
// to have checked: the connection arrives through the ingress, which proxies
// and decides nothing.
type attachHandler struct {
	useCase  *attachtask.UseCase
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

var _ http.Handler = &attachHandler{}

func NewAttachHandler(useCase *attachtask.UseCase, logger *slog.Logger) *attachHandler {
	return &attachHandler{
		useCase:  useCase,
		upgrader: terminal.Upgrader(),
		logger:   logger,
	}
}

// @Summary		Attach to an orchestrator task
// @Description	upgrades to a websocket carrying a command running inside the task
// @Tags			workload tasks
// @Param			uuid	path	string	true	"Task UUID"
// @Param			command	query	[]string	false	"The command to run; an interactive shell by default"
// @Success		101		{string}	string	"switching protocols"
// @Failure		404		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/attach [get]
func (h *attachHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &attachtask.Request{
		UUID:      r.PathValue("uuid"),
		Command:   r.URL.Query()["command"],
		TTY:       true,
		OwnerUUID: middleware.Subject(r.Context()),
	}

	session, validationErrors, err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such task", http.StatusNotFound)

		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "could not attach to a task", "error", err)
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

		// nobody ever attached to it, so nothing is going to end it either.
		go h.end(session)

		h.logger.ErrorContext(r.Context(), "failed to upgrade an attach connection", "error", err)

		return
	}

	terminal.Pump(conn, session, h.logger)

	// the client is gone. What it left running has nothing to show its output
	// to and no way back to it, so it is ended rather than left in the
	// task for as long as the task lives.
	go h.end(session)
}

// end stops what the client left running. Detached from the request, which is
// over: the command is given its grace period after the person has gone.
func (h *attachHandler) end(session task.ExecSession) {
	ctx, cancel := context.WithTimeout(context.Background(), endWait)
	defer cancel()

	if err := session.End(ctx); err != nil {
		h.logger.Warn("could not end a command left running in a task", "error", err)
	}
}
