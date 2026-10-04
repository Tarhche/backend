package runs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"syscall"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// terminalMarker names the environment variable every exec carries, holding
// its ID, so that the exec and everything it started can be found again from
// inside the guest when it is ended. It is the one docker's execs carry, so a
// task's terminal is ended the same way on either runtime.
const terminalMarker = "WORKLOAD_TERMINAL_SESSION"

// Exec is a command running inside a run, besides its main process: a
// terminal, most often.
//
// Its output is read for as long as it runs, whether or not anyone is reading
// it from here: Detach lets go of the output without ending the command, as
// closing a docker exec's connection does, and ending it is EndExec's.
type Exec struct {
	id      string
	process Process
	stdin   io.WriteCloser

	output   chan Output
	detached chan struct{}
	detach   sync.Once

	// done is closed once the command has ended, and code is how it ended
	// from then on.
	done chan struct{}
	code int

	mu   sync.Mutex
	sent syscall.Signal
}

// Output is a piece of what an exec wrote, with the stream it came from:
// api.OutputStdout or api.OutputStderr. With a terminal, everything is
// stdout.
type Output struct {
	Stream byte
	Data   []byte
}

// ID is what the exec is ended by.
func (e *Exec) ID() string {
	return e.id
}

// Output is what the command writes, in order. It is closed once the command
// has ended.
func (e *Exec) Output() <-chan Output {
	return e.output
}

// Done is closed once the command has ended.
func (e *Exec) Done() <-chan struct{} {
	return e.done
}

// ExitCode is how the command ended, numbered as docker numbers it. It is
// only meaningful once Done is closed.
func (e *Exec) ExitCode() int {
	<-e.done

	return e.code
}

// Write feeds the command's standard input.
func (e *Exec) Write(p []byte) (int, error) {
	if e.stdin == nil {
		return 0, errors.New("the command has no standard input")
	}

	return e.stdin.Write(p)
}

// CloseStdin closes the command's standard input, which is how a command that
// reads it to its end is told there is no more.
func (e *Exec) CloseStdin() error {
	if e.stdin == nil {
		return nil
	}

	return e.stdin.Close()
}

// Resize changes the size of the command's terminal.
func (e *Exec) Resize(ctx context.Context, rows, cols uint16) error {
	return e.process.Resize(ctx, rows, cols)
}

// Signal sends a signal, numbered as Linux numbers it, to the command's
// process group.
func (e *Exec) Signal(ctx context.Context, signal syscall.Signal) error {
	e.mu.Lock()
	if e.sent != sigKill {
		e.sent = signal
	}
	e.mu.Unlock()

	return e.process.Signal(ctx, signal)
}

// Detach lets go of the command's output: nobody is reading it any more. The
// command carries on, and what it writes from then on is dropped.
func (e *Exec) Detach() {
	e.detach.Do(func() {
		close(e.detached)
	})
}

// forward hands a piece of output to whoever is reading it, or drops it once
// nobody is.
func (e *Exec) forward(stream byte, data []byte) {
	select {
	case e.output <- Output{Stream: stream, Data: data}:
	case <-e.detached:
	}
}

// Exec starts a command inside a running run. The command runs as the image's
// user, in the run's environment and working directory unless it says
// otherwise, and with a standard input.
func (s *Supervisor) Exec(ctx context.Context, id string, request api.ExecRequest) (*Exec, error) {
	if err := validateExec(request); err != nil {
		return nil, err
	}

	s.mu.Lock()

	if err := s.readable(); err != nil {
		s.mu.Unlock()

		return nil, err
	}

	r, found := s.runs[id]
	if !found {
		s.mu.Unlock()

		return nil, notFound(id)
	}

	l := r.live
	if l == nil || r.record.State != api.StateRunning {
		s.mu.Unlock()

		return nil, notRunning(id)
	}

	s.mu.Unlock()

	execID, err := newID()
	if err != nil {
		return nil, err
	}

	command := Command{
		Argv:    slices.Clone(request.Command),
		Env:     append(slices.Clone(request.Env), terminalMarker+"="+execID),
		Workdir: request.WorkDir,
		TTY:     request.TTY,
		Stdin:   true,
		Rows:    request.Rows,
		Cols:    request.Cols,
	}

	// the command outlives the request that started it, so only starting it
	// is bounded, and by the supervisor rather than by the client.
	startCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.config.CallTimeout)
	defer cancel()

	started, err := l.sandbox.Exec(startCtx, command)
	if err != nil {
		// a run that ended in the meantime took its VM with it.
		s.mu.Lock()
		ended := r.live != l
		s.mu.Unlock()

		if ended {
			return nil, notRunning(id)
		}

		return nil, fmt.Errorf("the command could not be started: %w", err)
	}

	process := &onceClosed{Process: started}

	pending, last, outcome := awaitStart(startCtx, process)

	switch outcome {
	case outcomeStarted:
	case outcomeFailed:
		_ = process.Close()

		return nil, newError(api.CodeInvalid, "the command could not be started: %s", spawnFailureMessage(last))
	case outcomeLost:
		_ = process.Close()

		return nil, notRunning(id)
	default:
		_ = process.Close()

		return nil, fmt.Errorf("the command did not start within %s", s.config.CallTimeout)
	}

	// microsandbox starts every terminal at 24 by 80, whatever it was asked
	// for, so the size asked for is set once the terminal is there.
	if request.TTY && request.Rows > 0 && request.Cols > 0 {
		if err := process.Resize(startCtx, request.Rows, request.Cols); err != nil {
			s.logger.Warn("a terminal could not be given its size", "run", id, "error", err)
		}
	}

	e := &Exec{
		id:       execID,
		process:  process,
		stdin:    process.Stdin(),
		output:   make(chan Output, 64),
		detached: make(chan struct{}),
		done:     make(chan struct{}),
	}

	s.mu.Lock()

	// the run may have ended while the command was starting, and its VM
	// with it; and a service shutting down starts nothing it would have to
	// wait for.
	if r.live != l || s.closing {
		s.mu.Unlock()

		_ = process.Close()

		return nil, notRunning(id)
	}

	r.execs[execID] = e
	s.background.Add(1)

	s.mu.Unlock()

	go s.drain(e, pending)

	return e, nil
}

// drain reads an exec's events until the command ends, handing its output on
// while anyone is reading it.
func (s *Supervisor) drain(e *Exec, pending []Event) {
	defer s.background.Done()

	var (
		last  Event
		ended bool
	)

	consume := func(event Event) bool {
		switch event.Kind {
		case EventStdout:
			e.forward(api.OutputStdout, event.Data)
		case EventStderr:
			e.forward(api.OutputStderr, event.Data)
		case EventExited, EventFailed, EventLost:
			last, ended = event, true
		}

		return ended
	}

	for _, event := range pending {
		if consume(event) {
			break
		}
	}

	if !ended {
		for event := range e.process.Events() {
			if consume(event) {
				break
			}
		}
	}

	e.mu.Lock()
	sent := e.sent
	e.mu.Unlock()

	e.code = endingOf(last, ended, sent, false).code

	// the command has ended, so closing its handle only lets go of it.
	_ = e.process.Close()

	close(e.output)
	close(e.done)
}

// EndExec ends a command Exec started, and everything it started in turn, the
// way a container's exec is ended: it is given a moment to finish by itself,
// then asked to stop, and then killed. A command that has already ended is
// left alone.
//
// Signals reach the command's process group, but not whatever it started in a
// session of its own, such as a daemon a terminal left behind. Those still
// carry the exec's marker in their environment, so each signal is also swept
// across every process in the guest that carries it.
func (s *Supervisor) EndExec(ctx context.Context, id, execID string) error {
	s.mu.Lock()

	if err := s.readable(); err != nil {
		s.mu.Unlock()

		return err
	}

	r, found := s.runs[id]
	if !found {
		s.mu.Unlock()

		return notFound(id)
	}

	e, found := r.execs[execID]
	if !found {
		s.mu.Unlock()

		return newError(api.CodeNotFound, "run %s has no exec %s", id, execID)
	}

	l := r.live

	s.mu.Unlock()

	defer s.forgetExec(r, e)

	if waitFor(e.done, s.config.ExecEndGrace) {
		return nil
	}

	s.signalExec(l, e, sigTerm)

	if waitFor(e.done, s.config.ExecKillGrace) {
		return nil
	}

	s.signalExec(l, e, sigKill)

	// closing the command's handle ends whatever of it is left.
	if !waitFor(e.done, s.config.KillGrace) {
		_ = e.process.Close()
	}

	return nil
}

// signalExec sends a signal to an exec's process group, and sweeps it across
// everything in the guest that carries the exec's marker.
func (s *Supervisor) signalExec(l *live, e *Exec, signal syscall.Signal) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.CallTimeout)
	defer cancel()

	_ = e.Signal(ctx, signal)

	if l != nil {
		s.sweep(ctx, l.sandbox, e.id, signal)
	}
}

// sweep signals every process in a guest that carries an exec's marker, from
// inside the guest, which is the only place they can be named.
func (s *Supervisor) sweep(ctx context.Context, sandbox Sandbox, execID string, signal syscall.Signal) {
	process, err := sandbox.Exec(ctx, Command{Argv: []string{"/bin/sh", "-c", sweepScript(execID, signal)}})
	if err != nil {
		s.logger.Warn("an exec's processes could not be swept", "exec", execID, "error", err)

		return
	}

	// waited for to the end, so closing it only lets go of it. An image with
	// no shell cannot be swept, which leaves behind only what the process
	// group's signal did not reach.
	defer process.Close()

	for {
		select {
		case event, open := <-process.Events():
			if !open {
				return
			}

			switch event.Kind {
			case EventExited, EventFailed, EventLost:
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// forgetExec lets go of an exec that has been ended.
func (s *Supervisor) forgetExec(r *run, e *Exec) {
	e.Detach()

	s.mu.Lock()
	defer s.mu.Unlock()

	if r.execs[e.id] == e {
		delete(r.execs, e.id)
	}
}

// sweepScript signals every process carrying an exec's marker, and reports
// through its exit status whether it found any. It is the script a container's
// exec is ended with.
func sweepScript(execID string, signal syscall.Signal) string {
	return fmt.Sprintf(`found=1
for p in /proc/[0-9]*; do
	grep -qs '%s=%s' "$p/environ" || continue
	kill -%d "${p#/proc/}" 2>/dev/null && found=0
done
exit $found`, terminalMarker, execID, int(signal))
}
