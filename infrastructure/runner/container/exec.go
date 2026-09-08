package container

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gofrs/uuid/v5"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Exec starts a command inside a running container and hands back the stream it
// runs on. Closing the session releases that stream; ending it is what stops
// the command.
func (m *DockerManager) Exec(ctx context.Context, containerUUID string, options container.ExecOptions) (container.ExecSession, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.exec",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	// the mark travels in the environment, so everything the command starts
	// carries it too and a whole session can be ended together.
	terminalID, err := uuid.NewV7()
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	created, err := m.client.ContainerExecCreate(ctx, containerUUID, containerTypes.ExecOptions{
		Cmd:          options.Command,
		Env:          append(options.Env, terminalMarker+"="+terminalID.String()),
		WorkingDir:   options.WorkDir,
		Tty:          options.TTY,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	// the attach outlives this call, so it must not be tied to a context that
	// ends with it. Closing the session is what releases the connection.
	attached, err := m.client.ContainerExecAttach(context.WithoutCancel(ctx), created.ID, containerTypes.ExecAttachOptions{
		Tty: options.TTY,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	m.logger.Info("exec started", "containerUUID", containerUUID, "execID", created.ID)

	return &execSession{
		client:      m.client,
		execID:      created.ID,
		containerID: containerUUID,
		terminalID:  terminalID.String(),
		connection:  attached.Conn,
		reader:      attached.Reader,
	}, nil
}

// execSession is a command running inside a container: reading takes its
// output, writing feeds its input, and closing tears it down.
//
// With a TTY there is one stream rather than two, which is what makes the
// reader usable directly and is why an attached terminal asks for one.
type execSession struct {
	client *client.Client
	execID string

	// containerID and terminalID are what the session is ended by: the
	// container to look inside, and the mark its processes carry.
	containerID string
	terminalID  string

	connection io.WriteCloser
	reader     io.Reader

	shut sync.Once
	err  error
}

var _ container.ExecSession = &execSession{}

func (s *execSession) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *execSession) Write(p []byte) (int, error) {
	return s.connection.Write(p)
}

// Resize tells the command's terminal how big it now is, so what it draws fits
// the window the client is showing it in.
func (s *execSession) Resize(ctx context.Context, rows uint, cols uint) error {
	return s.client.ContainerExecResize(ctx, s.execID, containerTypes.ResizeOptions{
		Height: rows,
		Width:  cols,
	})
}

// Close releases the stream the command runs on. It is safe to call more than
// once, and from a goroutine other than the one reading: closing the connection
// is what releases a caller parked in Read.
//
// What was running carries on: docker leaves an exec running when its client
// goes away. End is what stops it.
func (s *execSession) Close() error {
	s.shut.Do(func() {
		s.err = s.connection.Close()
	})

	return s.err
}

// how a command left without a client is ended: it is given a moment to finish
// on its own, then asked to stop, and then stopped for good.
const (
	terminationGrace = 5 * time.Second
	killGrace        = 5 * time.Second
)

// terminalMarker names the environment variable every command started by Exec
// carries, so that the command and everything it started can be found again
// from inside the container when there is nobody left attached to it.
//
// Docker has no way to end an exec: closing the connection leaves whatever was
// running behind, and there is no reattaching to it. The processes themselves
// are the only handle left, and this is what marks them.
const terminalMarker = "RUNNER_TERMINAL_SESSION"

// End stops the command and everything it started.
func (s *execSession) End(ctx context.Context) error {
	select {
	case <-time.After(terminationGrace):
	case <-ctx.Done():
		return ctx.Err()
	}

	signalled, err := s.signal(ctx, "TERM")
	if err != nil || !signalled {
		// nothing was there to signal: the command ended on its own, which is
		// the outcome this was after.
		return err
	}

	select {
	case <-time.After(killGrace):
	case <-ctx.Done():
		return ctx.Err()
	}

	_, err = s.signal(ctx, "KILL")

	return err
}

// signal sends one signal to what is left of the session, and reports whether
// anything was still there to take it.
//
// It runs inside the container because that is the only place the session's
// processes can be named: the pids docker reports are the daemon's, and the
// worker is not on that host.
func (s *execSession) signal(ctx context.Context, signal string) (bool, error) {
	created, err := s.client.ContainerExecCreate(ctx, s.containerID, containerTypes.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", signalScript(s.terminalID, signal)},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return false, err
	}

	attached, err := s.client.ContainerExecAttach(ctx, created.ID, containerTypes.ExecAttachOptions{})
	if err != nil {
		return false, err
	}
	defer attached.Close()

	// reading to the end is what waits for it.
	if _, err := io.Copy(io.Discard, attached.Reader); err != nil {
		return false, err
	}

	inspected, err := s.client.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return false, err
	}

	// an image with no shell cannot be swept, which is a command left running
	// rather than a failure of the container: it is reported, not retried.
	return inspected.ExitCode == 0, nil
}

// signalScript signals every process carrying the session's mark, and reports
// through its exit status whether it found any.
func signalScript(terminalID string, signal string) string {
	return fmt.Sprintf(`found=1
for p in /proc/[0-9]*; do
	grep -qs '%s=%s' "$p/environ" || continue
	kill -%s "${p#/proc/}" 2>/dev/null && found=0
done
exit $found`, terminalMarker, terminalID, signal)
}
