// Package vm serves the VMs this node holds: their terminals.
package vm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/trace"

	attachvm "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/attachVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/orchestrator/api/terminal"
)

// attachHandler carries a shell in a VM over a websocket, as the terminal
// package speaks it: the same protocol as a task's terminal.
//
// Who is asking comes from the token the middleware verified, and whose VM it
// is comes off the VM itself. Nothing in between is trusted to have checked:
// the connection arrives through the ingress, which proxies and decides
// nothing.
type attachHandler struct {
	useCase  *attachvm.UseCase
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

var _ http.Handler = &attachHandler{}

func NewAttachHandler(useCase *attachvm.UseCase, logger *slog.Logger) *attachHandler {
	return &attachHandler{useCase: useCase, upgrader: terminal.Upgrader(), logger: logger}
}

// @Summary		Open a terminal in a VM
// @Description	upgrades to a websocket carrying a shell in the VM, for its owner alone
// @Tags			workload vms
// @Param			uuid	path	string	true	"VM UUID"
// @Success		101		{string}	string	"switching protocols"
// @Failure		400		{object}	map[string]interface{}
// @Failure		401		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/vms/{uuid}/attach [get]
func (h *attachHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &attachvm.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: middleware.Subject(r.Context()),
	}

	session, validationErrors, err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such vm", http.StatusNotFound)

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
		// nobody is attached to the shell, so it is ended now.
		_ = session.Close()

		h.logger.ErrorContext(r.Context(), "failed to upgrade a vm's terminal", "error", err)

		return
	}

	// closing the session ends the shell, which is what a terminal nobody is
	// attached to any more is for.
	terminal.Pump(conn, shell{session: session}, h.logger)
}

// shell is a command in a VM as a terminal reads and writes it. It runs on a
// terminal, so everything it says is on its output.
type shell struct {
	session vm.ExecSession
}

var _ terminal.Session = shell{}

func (s shell) Read(p []byte) (int, error) {
	return s.session.Stdout().Read(p)
}

func (s shell) Write(p []byte) (int, error) {
	return s.session.Stdin().Write(p)
}

func (s shell) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.session.Resize(ctx, rows, cols)
}

func (s shell) Close() error {
	return s.session.Close()
}

var _ io.ReadWriteCloser = shell{}
