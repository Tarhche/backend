// Package kinds serves what the ingress carries to this node for the kinds
// it runs: the streams of a kind whose node strategy serves them, its
// terminal say, and the ports of a kind with endpoints, each under the kind's
// own plural.
package kinds

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/trace"

	attachresource "github.com/khanzadimahdi/testproject/application/workload/orchestrator/attachResource"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/terminal"
)

// attachHandler carries a stream action of one kind over a websocket, as the
// terminal package speaks it: the protocol a VM's and a task's terminals
// speak, so one terminal on the page opens any of them.
//
// Who is asking comes from the token the middleware verified, and whose the
// resource is comes off the resource itself, which the kind's node strategy
// reads. Nothing in between is trusted to have checked: the connection
// arrives through the ingress, which proxies and decides nothing.
type attachHandler struct {
	useCase  *attachresource.UseCase
	kind     string
	action   string
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

var _ http.Handler = &attachHandler{}

// NewAttachHandler serves the stream action of that name of the kind of that
// name.
func NewAttachHandler(useCase *attachresource.UseCase, kindName string, action string, logger *slog.Logger) *attachHandler {
	return &attachHandler{useCase: useCase, kind: kindName, action: action, upgrader: terminal.Upgrader(), logger: logger}
}

// @Summary		Open a stream in a resource
// @Description	upgrades to a websocket carrying a stream action, a terminal say, in a resource of a kind this node runs, for its owner alone
// @Tags			workload kinds
// @Param			plural	path	string	true	"The kind's plural"
// @Param			uuid	path	string	true	"The resource's UUID"
// @Param			action	path	string	true	"The stream action"
// @Success		101		{string}	string	"switching protocols"
// @Failure		400		{object}	map[string]interface{}
// @Failure		401		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/{plural}/{uuid}/{action} [get]
func (h *attachHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &attachresource.Request{
		Kind:      h.kind,
		Action:    h.action,
		UUID:      r.PathValue("uuid"),
		OwnerUUID: middleware.Subject(r.Context()),
	}

	session, validationErrors, err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such "+h.kind, http.StatusNotFound)

		return
	case errors.Is(err, kind.ErrUnreachable):
		// theirs, and there, and not to be opened now: not running, say.
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)

		return
	case errors.Is(err, kind.ErrInvalidPayload):
		http.Error(rw, err.Error(), http.StatusBadRequest)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	case len(validationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(rw).Encode(map[string]any{"errors": validationErrors})

		return
	}

	conn, err := h.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		// nobody is attached to the stream, so it is ended now.
		_ = session.Close()

		h.logger.ErrorContext(r.Context(), "failed to upgrade a stream", "error", err, "kind", h.kind, "action", h.action)

		return
	}

	// closing the session ends the stream, which is what a terminal nobody is
	// attached to any more is for.
	terminal.Pump(conn, stream{session: session}, h.logger)
}

// stream is a session as a terminal reads and writes it. It runs on a
// terminal, so everything it says is on its output.
type stream struct {
	session kind.Session
}

var _ terminal.Session = stream{}

func (s stream) Read(p []byte) (int, error) {
	return s.session.Stdout().Read(p)
}

func (s stream) Write(p []byte) (int, error) {
	return s.session.Stdin().Write(p)
}

func (s stream) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.session.Resize(ctx, rows, cols)
}

func (s stream) Close() error {
	return s.session.Close()
}
