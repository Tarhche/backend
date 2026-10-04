package microsandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// requestWait bounds waiting for the ExecRequest a client opens with.
	requestWait = 30 * time.Second

	// writeWait bounds one write to the client, so a stalled one cannot hold
	// the writer forever.
	writeWait = 10 * time.Second

	// closeWait is how long a client is given to close its end once the
	// command has ended and the service has closed its own.
	closeWait = 5 * time.Second

	// maxFrame is the most a client's frame may hold.
	maxFrame = 1 << 20
)

// execHandler carries a command running inside a run over a websocket, in the
// frames ExecRequest describes.
//
// Closing the websocket lets go of the command and does not end it, as
// closing a container exec's connection does not: ending it is
// endExecHandler's, which gives it a grace period, so a terminal whose
// connection dropped is not killed with the connection.
type execHandler struct {
	supervisor *runs.Supervisor
	upgrader   websocket.Upgrader
	logger     *slog.Logger
}

var _ http.Handler = &execHandler{}

func NewExecHandler(supervisor *runs.Supervisor, logger *slog.Logger) *execHandler {
	return &execHandler{
		supervisor: supervisor,
		// who is asking is settled by the certificate the client presented,
		// not by an origin: the clients are orchestrators, not browsers.
		upgrader: websocket.Upgrader{
			CheckOrigin:  func(*http.Request) bool { return true },
			Subprotocols: []string{api.ExecSubprotocol},
		},
		logger: logger,
	}
}

func (h *execHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	id := r.PathValue(api.WildcardRun)

	// what can be refused before the upgrade is, with a status and an
	// ErrorResponse rather than a frame.
	if !slices.Contains(websocket.Subprotocols(r), api.ExecSubprotocol) {
		writeError(rw, r, &api.Error{
			Code:    api.CodeInvalid,
			Message: fmt.Sprintf("an exec is carried in %s frames, which the client did not offer", api.ExecSubprotocol),
		})

		return
	}

	run, err := h.supervisor.Get(id)
	if err != nil {
		writeError(rw, r, err)

		return
	}

	if run.State != api.StateRunning {
		writeError(rw, r, &api.Error{Code: api.CodeNotRunning, Message: fmt.Sprintf("run %s is not running", id)})

		return
	}

	conn, err := h.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		// the upgrader has answered already.
		return
	}
	defer conn.Close()

	conn.SetReadLimit(maxFrame)

	request, err := readRequest(conn)
	if err != nil {
		h.fail(conn, err)

		return
	}

	// the command outlives this request, so it is started with the
	// supervisor's own deadline rather than the request's.
	exec, err := h.supervisor.Exec(context.WithoutCancel(r.Context()), id, request)
	if err != nil {
		h.fail(conn, err)

		return
	}
	defer exec.Detach()

	if err := writeControl(conn, api.Control{Type: api.ControlStarted, ExecID: exec.ID()}); err != nil {
		return
	}

	h.pump(conn, exec)
}

// readRequest reads the ExecRequest a client opens with.
func readRequest(conn *websocket.Conn) (api.ExecRequest, error) {
	_ = conn.SetReadDeadline(time.Now().Add(requestWait))

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return api.ExecRequest{}, &api.Error{Code: api.CodeInvalid, Message: fmt.Sprintf("no request arrived: %v", err)}
	}

	_ = conn.SetReadDeadline(time.Time{})

	var request api.ExecRequest

	if messageType != websocket.TextMessage {
		return request, &api.Error{Code: api.CodeInvalid, Message: "the first frame has to be an ExecRequest, as text"}
	}

	if err := json.Unmarshal(payload, &request); err != nil {
		return request, &api.Error{Code: api.CodeInvalid, Message: fmt.Sprintf("the first frame is not an ExecRequest: %v", err)}
	}

	return request, nil
}

// pump carries the command's output to the client, and the client's input and
// controls to the command, until the command ends or the client goes away.
//
// Output goes out from one goroutine, which is the only one that writes
// frames, so writes never interleave. It ends with the exit frame once the
// command has ended, and closes the connection, which ends the read loop.
func (h *execHandler) pump(conn *websocket.Conn, exec *runs.Exec) {
	gone := make(chan struct{})
	written := make(chan struct{})

	go func() {
		defer close(written)

		h.writeOutput(conn, exec, gone)
	}()

	defer func() {
		close(gone)
		<-written
	}()

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			if _, err := exec.Write(payload); err != nil {
				h.logger.Warn("an exec's input could not be written", "exec", exec.ID(), "error", err)
			}

		case websocket.TextMessage:
			var control api.Control
			if err := json.Unmarshal(payload, &control); err != nil {
				continue
			}

			h.control(exec, control)
		}
	}
}

// control does what a client's control frame asks. One that makes no sense is
// ignored, as one of a type it does not know is.
func (h *execHandler) control(exec *runs.Exec, control api.Control) {
	ctx, cancel := context.WithTimeout(context.Background(), writeWait)
	defer cancel()

	var err error

	switch control.Type {
	case api.ControlResize:
		if control.Rows > 0 && control.Cols > 0 {
			err = exec.Resize(ctx, control.Rows, control.Cols)
		}
	case api.ControlCloseStdin:
		err = exec.CloseStdin()
	case api.ControlSignal:
		if control.Signal > 0 {
			err = exec.Signal(ctx, syscall.Signal(control.Signal))
		}
	}

	if err != nil {
		h.logger.Warn("an exec's control could not be applied", "exec", exec.ID(), "control", control.Type, "error", err)
	}
}

// writeOutput writes the command's output, then how it ended, and closes the
// connection. It stops early once the client has gone.
func (h *execHandler) writeOutput(conn *websocket.Conn, exec *runs.Exec, gone <-chan struct{}) {
	for {
		select {
		case output, open := <-exec.Output():
			if !open {
				code := exec.ExitCode()

				if err := writeControl(conn, api.Control{Type: api.ControlExit, Code: &code}); err != nil {
					return
				}

				closeConnection(conn)

				return
			}

			frame := append([]byte{output.Stream}, output.Data...)

			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		case <-gone:
			return
		}
	}
}

// fail tells the client why the command could not be run, and closes.
func (h *execHandler) fail(conn *websocket.Conn, err error) {
	var apiError *api.Error
	if !errors.As(err, &apiError) {
		apiError = &api.Error{Code: api.CodeInternal, Message: err.Error()}
	}

	if writeErr := writeControl(conn, api.Control{Type: api.ControlError, Error: apiError}); writeErr == nil {
		closeConnection(conn)
	}
}

func writeControl(conn *websocket.Conn, control api.Control) error {
	payload, err := json.Marshal(control)
	if err != nil {
		return err
	}

	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))

	return conn.WriteMessage(websocket.TextMessage, payload)
}

// closeConnection sends a close frame and gives the client a moment to close
// its end, after which reading gives up. The deadline is set on the
// connection underneath, which unblocks a read already waiting on it.
func closeConnection(conn *websocket.Conn) {
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(writeWait),
	)

	_ = conn.UnderlyingConn().SetReadDeadline(time.Now().Add(closeWait))
}

// endExecHandler ends a command an exec started.
type endExecHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &endExecHandler{}

func NewEndExecHandler(supervisor *runs.Supervisor) *endExecHandler {
	return &endExecHandler{supervisor: supervisor}
}

func (h *endExecHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.supervisor.EndExec(r.Context(), r.PathValue(api.WildcardRun), r.PathValue(api.WildcardExec))
	if err != nil {
		writeError(rw, r, err)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
