package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// writeWait bounds one frame written to the service, so a stalled
	// connection cannot hold whoever is writing forever.
	writeWait = 10 * time.Second

	// closeWait bounds saying goodbye before the connection is let go.
	closeWait = time.Second
)

// Exec starts a command inside a running run and hands back the stream it runs
// on. Closing the session releases that stream; ending it is what stops the
// command.
func (r *Runtime) Exec(ctx context.Context, runID string, options task.ExecOptions) (task.ExecSession, error) {
	ctx, span := r.client.span(ctx, "task.exec", attribute.String("task.id", runID))
	defer span.End()

	session, err := r.exec(ctx, runID, options)
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	r.client.logger.InfoContext(ctx, "exec started", "runID", runID, "execID", session.execID)

	return session, nil
}

// exec opens the exec's websocket, asks for the command, and waits for the
// service to say it started.
func (r *Runtime) exec(ctx context.Context, runID string, options task.ExecOptions) (*execSession, error) {
	if err := r.client.verify(ctx); err != nil {
		return nil, err
	}

	_, location, err := r.client.locate(api.RouteExec, runPath(runID))
	if err != nil {
		return nil, err
	}

	location.Scheme = "wss"

	header := make(http.Header)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))

	// the handshake is bounded, and the session is not: once the command has
	// started, it lasts as long as the command, whatever happens to the call
	// that started it.
	handshake, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, response, err := r.client.exec.DialContext(handshake, location.String(), header)
	if err != nil {
		if response != nil && response.StatusCode >= http.StatusBadRequest {
			return nil, refusal(response)
		}

		return nil, r.client.unreachable(ctx, err)
	}

	if conn.Subprotocol() != api.ExecSubprotocol {
		_ = conn.Close()

		return nil, fmt.Errorf("workload-microsandbox would not speak %s over an exec", api.ExecSubprotocol)
	}

	started, err := ask(handshake, conn, api.ExecRequest{
		Command: options.Command,
		TTY:     options.TTY,
		Env:     options.Env,
		WorkDir: options.WorkDir,
	})
	if err != nil {
		_ = conn.Close()

		return nil, err
	}

	return &execSession{
		client: r.client,
		runID:  runID,
		execID: started,
		conn:   conn,
	}, nil
}

// ask sends the command, and reads whether it started and under which ID: the
// first frame each way, as text.
func ask(ctx context.Context, conn *websocket.Conn, command api.ExecRequest) (string, error) {
	deadline, _ := ctx.Deadline()

	_ = conn.SetWriteDeadline(deadline)
	if err := conn.WriteJSON(command); err != nil {
		return "", fmt.Errorf("workload-microsandbox could not be asked for the command: %w", err)
	}

	_ = conn.SetReadDeadline(deadline)

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return "", fmt.Errorf("workload-microsandbox did not say whether the command started: %w", err)
	}

	var answer api.Control
	if messageType != websocket.TextMessage || json.Unmarshal(payload, &answer) != nil {
		return "", errors.New("workload-microsandbox answered the command with something other than whether it started")
	}

	switch answer.Type {
	case api.ControlStarted:
		if len(answer.ExecID) == 0 {
			return "", errors.New("workload-microsandbox started the command and did not say which it was")
		}
	case api.ControlError:
		if answer.Error == nil {
			return "", errors.New("workload-microsandbox could not start the command, and did not say why")
		}

		return "", refused(*answer.Error)
	default:
		return "", fmt.Errorf("workload-microsandbox answered the command with %q rather than whether it started", answer.Type)
	}

	// from here on the session lasts as long as the command, so nothing
	// bounds reading it or, beyond one frame at a time, writing to it.
	_ = conn.SetReadDeadline(time.Time{})
	_ = conn.SetWriteDeadline(time.Time{})

	return answer.ExecID, nil
}

// execSession is a command running inside a run: reading takes its output,
// writing feeds its input, and closing lets go of the stream it runs on.
//
// One goroutine reads and any may write, as with docker's attached stream.
// Closing is safe from any goroutine, and releases one parked in Read.
type execSession struct {
	client *Client
	runID  string
	execID string

	conn *websocket.Conn

	// frame is the rest of the output frame a Read is in the middle of, and
	// ended what every Read answers once the stream is over.
	frame io.Reader
	ended error

	// writing keeps input and control frames from going out on top of each
	// other: the connection takes one writer at a time.
	writing sync.Mutex

	closed  atomic.Bool
	closing sync.Once
	shut    error
}

var _ task.ExecSession = &execSession{}

// Read takes the command's output: its stdout and stderr together, in the order
// they were written, or its terminal when it has one. It is io.EOF once the
// command has exited and everything it wrote has been read.
func (s *execSession) Read(p []byte) (int, error) {
	for {
		if s.ended != nil {
			return 0, s.ended
		}

		if len(p) == 0 {
			return 0, nil
		}

		if s.frame == nil {
			s.ended = s.next()

			continue
		}

		n, err := s.frame.Read(p)
		if err != nil {
			s.frame = nil

			if !errors.Is(err, io.EOF) {
				s.ended = s.broken(err)
			}
		}

		if n > 0 {
			return n, nil
		}
	}
}

// next reads frames until there is output to hand over, which it leaves in
// s.frame, or the stream is over, which it says why.
func (s *execSession) next() error {
	messageType, reader, err := s.conn.NextReader()
	if err != nil {
		return s.broken(err)
	}

	switch messageType {
	case websocket.BinaryMessage:
		// the first byte says which stream the rest came from. Reading takes
		// both, as reading docker's attached stream does, so it is dropped.
		var source [1]byte
		if _, err := io.ReadFull(reader, source[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return s.broken(err)
		}

		s.frame = reader

		return nil

	case websocket.TextMessage:
		var control api.Control
		if err := json.NewDecoder(reader).Decode(&control); err != nil {
			return nil
		}

		switch control.Type {
		case api.ControlExit:
			// what the command returned is not the session's to say: a
			// terminal's reader only needs to know there is no more.
			return io.EOF
		case api.ControlError:
			if control.Error == nil {
				return errors.New("the command's stream ended in an error the service did not explain")
			}

			return refused(*control.Error)
		}
	}

	// anything else is something a later service says and this client does
	// not know, and is read past.
	return nil
}

// broken is a stream that ended without the command's exit. Once the session
// was closed that is simply its end, since there is nothing more to read; until
// then it is the stream breaking under a command that may still be running.
func (s *execSession) broken(err error) error {
	if s.closed.Load() {
		return io.EOF
	}

	return fmt.Errorf("the command's stream ended before the command did: %w", err)
}

// Write feeds the command's stdin.
func (s *execSession) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if err := s.write(context.Background(), websocket.BinaryMessage, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Resize tells the command's terminal how big it now is, so what it draws fits
// the window the client is showing it in.
func (s *execSession) Resize(ctx context.Context, rows uint, cols uint) error {
	control, err := json.Marshal(api.Control{
		Type: api.ControlResize,
		Rows: dimension(rows),
		Cols: dimension(cols),
	})
	if err != nil {
		return err
	}

	return s.write(ctx, websocket.TextMessage, control)
}

// write sends one frame, bounded by writeWait or by ctx when it is sooner.
func (s *execSession) write(ctx context.Context, messageType int, payload []byte) error {
	deadline := time.Now().Add(writeWait)
	if sooner, ok := ctx.Deadline(); ok && sooner.Before(deadline) {
		deadline = sooner
	}

	s.writing.Lock()
	defer s.writing.Unlock()

	_ = s.conn.SetWriteDeadline(deadline)

	return s.conn.WriteMessage(messageType, payload)
}

// dimension is a terminal's size as the service takes it. No terminal is that
// many characters across, so a size past what it takes is the most it takes
// rather than one that wraps round to a small one.
func dimension(size uint) uint16 {
	return uint16(min(size, math.MaxUint16))
}

// Close lets go of the stream the command runs on. What was running carries
// on, as it does when a docker exec's client goes away: End is what stops it.
// It is safe to call more than once.
func (s *execSession) Close() error {
	s.closing.Do(func() {
		s.closed.Store(true)

		// a courtesy: the service lets the command run whether or not it
		// hears the connection was closed on purpose.
		_ = s.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(closeWait),
		)

		s.shut = s.conn.Close()
	})

	return s.shut
}

// End stops the command, and everything it started, once nobody is attached
// to it any more. The service gives it a moment to finish on its own, asks it
// to stop, and kills it if it will not. A command that has already finished,
// or a run that has gone, is left alone, which is what ending it was for.
func (s *execSession) End(ctx context.Context) error {
	ctx, span := s.client.span(ctx, "task.exec.end",
		attribute.String("task.id", s.runID),
		attribute.String("exec.id", s.execID),
	)
	defer span.End()

	err := s.client.call(ctx, request{
		route:     api.RouteEndExec,
		wildcards: map[string]string{api.WildcardRun: s.runID, api.WildcardExec: s.execID},
	}, nil)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	return trace.RecordError(span, err)
}
