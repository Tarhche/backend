package container

import (
	"context"
	"io"
	"sync"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Exec starts a command inside a running container and hands back the stream it
// runs on. Closing the session is what ends the command.
func (m *DockerManager) Exec(ctx context.Context, containerUUID string, options container.ExecOptions) (container.ExecSession, error) {
	ctx, span := m.tracer.Start(ctx, "docker.container.exec",
		oteltrace.WithAttributes(attribute.String("container.id", containerUUID)),
	)
	defer span.End()

	created, err := m.client.ContainerExecCreate(ctx, containerUUID, containerTypes.ExecOptions{
		Cmd:          options.Command,
		Env:          options.Env,
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
		client:     m.client,
		execID:     created.ID,
		connection: attached.Conn,
		reader:     attached.Reader,
	}, nil
}

// execSession is a command running inside a container: reading takes its
// output, writing feeds its input, and closing tears it down.
//
// With a TTY there is one stream rather than two, which is what makes the
// reader usable directly and is why an attached terminal asks for one.
type execSession struct {
	client     *client.Client
	execID     string
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

// Close ends the command. It is safe to call more than once, and from a
// goroutine other than the one reading: closing the connection is what releases
// a caller parked in Read.
func (s *execSession) Close() error {
	s.shut.Do(func() {
		s.err = s.connection.Close()
	})

	return s.err
}
