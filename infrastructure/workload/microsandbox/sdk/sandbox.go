//go:build microsandbox

package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// sandbox is a live handle on a running sandbox.
type sandbox struct {
	name   string
	handle *msb.Sandbox
	logger *slog.Logger
}

var _ runs.Sandbox = &sandbox{}

func (s *sandbox) Name() string {
	return s.name
}

func (s *sandbox) Exec(ctx context.Context, command runs.Command) (runs.Process, error) {
	if len(command.Argv) == 0 || command.Argv[0] == "" {
		return nil, errors.New("microsandbox: exec: there is no program to run")
	}

	options, err := execOptions(command)
	if err != nil {
		return nil, err
	}

	handle, err := s.handle.ExecStream(ctx, command.Argv[0], command.Argv[1:], options...)
	if err != nil {
		return nil, fmt.Errorf("microsandbox: exec %s in %s: %w", command.Argv[0], s.name, err)
	}

	// a nil interface, not a nil sink, when the command has no stdin
	var stdin io.WriteCloser
	if sink := handle.TakeStdin(); sink != nil {
		if command.Stdin {
			stdin = sink
		} else if err := sink.Close(); err != nil {
			// it reads an input that never ends, rather than an empty one
			s.logger.Warn("could not close the stdin of a command that has none", "sandbox", s.name, "program", command.Argv[0], "error", err)
		}
	}

	return newProcess(execStream{handle: handle}, stdin, command), nil
}

// Close lets go of the handle and its connection to the guest agent. The VM
// carries on, and so does every command started through it: a command ends
// when its own Process is closed, or when this process exits.
func (s *sandbox) Close() error {
	if err := s.handle.Close(); err != nil {
		return fmt.Errorf("microsandbox: let go of %s: %w", s.name, err)
	}

	return nil
}
