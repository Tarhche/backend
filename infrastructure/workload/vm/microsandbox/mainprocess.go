//go:build microsandbox

package microsandbox

import (
	"context"
	"sync/atomic"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"
)

// mainProcess is an instance's main process: its image's entrypoint and
// command, or what its spec replaced them with, run as an exec session the
// engine owns for as long as it runs.
//
// It is an exec session rather than the guest's init because that is what
// microsandbox reports an exit code for, and what files its output apart in
// the instance's log. The session is the vmhost's, so a vmhost that stops
// takes it along, and the instance with it.
type mainProcess struct {
	handle *msb.ExecHandle

	// ended is set when the engine ends the process itself, by stopping,
	// restarting or removing its instance: how it exits then is not how it
	// ended.
	ended atomic.Bool

	// done is closed once its session has ended.
	done chan struct{}
}

// end ends the process, as the engine's own doing.
//
// It is killed, and its session is not waited for: a kill reaches the
// process alone, and what it started may run on and keep the session open
// for as long as the VM runs. The code runner's script does: it runs a
// snippet under timeout, which puts itself and the snippet in a process group
// of their own. Stopping, restarting or removing an instance takes its VM
// down next, which ends what is left, and the session with it.
func (m *mainProcess) end() {
	m.ended.Store(true)

	select {
	case <-m.done:
		return
	default:
	}

	ctx, cancel := context.WithTimeout(context.Background(), killTimeout)
	defer cancel()

	_ = m.handle.Kill(ctx)
}

// startMain starts an instance's main process, which is the first thing exec'd
// into it on this boot, so its output is the boot's first session in the log.
//
// A process that cannot even be started has ended as a shell would say it
// did, which the instance keeps as its exit, and it is stopped.
func (e *engine) startMain(ctx context.Context, i *instance, sb *msb.Sandbox) error {
	r := i.current()

	var options []msb.ExecOption

	if env := envOf(r.Spec.Env); env != nil {
		options = append(options, msb.WithExecEnv(env))
	}

	if len(r.Spec.WorkingDir) > 0 {
		options = append(options, msb.WithExecCwd(r.Spec.WorkingDir))
	}

	handle, err := sb.ExecDefaultStream(ctx, options...)
	if err != nil {
		code := 126
		if msb.IsKind(err, msb.ErrNoDefaultCommand) {
			code = 127
		}

		return e.mainEnded(ctx, i, exit{Code: code, Reason: "its main process could not be started: " + err.Error(), At: time.Now()})
	}

	m := &mainProcess{handle: handle, done: make(chan struct{})}

	i.mu.Lock()
	i.main = m
	i.mu.Unlock()

	if err := e.update(i, func(r *record) { r.MainRunning = true }); err != nil {
		m.end()

		return err
	}

	go e.watchMain(i, m)

	return nil
}

// watchMain follows a main process to its end, and stops its instance when
// it ended on its own.
func (e *engine) watchMain(i *instance, m *mainProcess) {
	ended := exit{Code: -1}

	for {
		event, err := m.handle.Recv(context.Background())
		if err != nil {
			ended.Reason = "its main process's session ended: " + err.Error()

			break
		}

		if event.Kind == msb.ExecEventDone {
			break
		}

		switch event.Kind {
		case msb.ExecEventExited:
			ended.Code = event.ExitCode
		case msb.ExecEventFailed:
			ended.Code = 126
			ended.Reason = "its main process could not be started"

			if event.Failure != nil {
				ended.Code = spawnExitCode(event.Failure.Kind)
				ended.Reason += ": " + event.Failure.Message
			}
		}
	}

	_ = m.handle.Close()
	close(m.done)

	if m.ended.Load() {
		return
	}

	ended.At = time.Now()

	ctx := context.Background()
	if err := i.acquire(ctx); err != nil {
		return
	}
	defer i.release()

	// stopped, restarted or removed while it was ending: this process is
	// not what the instance is running now.
	i.mu.Lock()
	current := i.main == m
	if current {
		i.main = nil
	}
	i.mu.Unlock()

	if !current || m.ended.Load() {
		return
	}

	if err := e.mainEnded(ctx, i, ended); err != nil {
		e.logger.Warn("a vm whose main process ended could not be stopped", "vm", i.id, "error", err)
	}
}

// mainEnded keeps how an instance's main process ended and stops the
// instance, as a container stops with its process. It is called with the
// instance's turn held.
func (e *engine) mainEnded(ctx context.Context, i *instance, ended exit) error {
	if err := e.update(i, func(r *record) {
		r.MainRunning = false
		r.Exit = &ended
	}); err != nil {
		return err
	}

	e.halt(i)

	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return err
	}

	defer e.forget(i)

	if h == nil || !up(h.Status()) {
		return nil
	}

	return e.shutdownSandbox(ctx, i, h, endedStopTimeout)
}
