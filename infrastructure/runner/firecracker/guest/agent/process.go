//go:build linux

package agent

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// start runs the machine's task: for the first time, or again after it
// ended, which is how a task is restarted without its machine.
func (a *Agent) start(rw http.ResponseWriter, r *http.Request) {
	var process guest.Process
	if !decode(rw, r, &process) {
		return
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if a.config == nil {
		http.Error(rw, "the machine has not been told what it is", http.StatusConflict)

		return
	}

	if a.status.State == guest.StateRunning {
		http.Error(rw, "the task is already running", http.StatusConflict)

		return
	}

	if err := a.run(process); err != nil {
		a.logger.Error("the task could not be started", "error", err)
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	answer(rw, a.status)
}

// run starts the task, inside its root and inside its cgroup, with what it
// writes going to the machine's log. The lock is held.
func (a *Agent) run(process guest.Process) error {
	if len(process.Args) == 0 {
		return fmt.Errorf("there is no command to run")
	}

	// whatever an earlier run left behind goes before this one starts.
	if err := removeCgroup(taskCgroup, emptyTimeout); err != nil {
		return err
	}

	who, err := resolveUser(rootDir, process.User)
	if err != nil {
		return err
	}

	env := withDefaults(process.Env, who.home)
	path, _ := envValue(env, "PATH")

	command, err := lookPath(rootDir, process.Args[0], path)
	if err != nil {
		return err
	}

	cgroup, err := openCgroup(mainCgroup)
	if err != nil {
		return err
	}
	defer cgroup.Close()

	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer stdin.Close()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return err
	}

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		stdoutReader.Close()
		stdoutWriter.Close()

		return err
	}

	attributes := &os.ProcAttr{
		Dir:   workingDir(process.WorkingDir),
		Env:   env,
		Files: []*os.File{stdin, stdoutWriter, stderrWriter},
		Sys: &syscall.SysProcAttr{
			Chroot:      rootDir,
			Credential:  &syscall.Credential{Uid: who.uid, Gid: who.gid, Groups: who.groups},
			Setsid:      true,
			UseCgroupFD: true,
			CgroupFD:    int(cgroup.Fd()),
		},
	}

	pid, ended, err := a.reaper.spawn(func() (int, error) {
		started, err := os.StartProcess(command, process.Args, attributes)
		if err != nil {
			return 0, err
		}

		pid := started.Pid
		_ = started.Release()

		return pid, nil
	})

	// the task holds the writing ends now; the agent keeps only the reading
	// ones, so the task ending is what ends them.
	stdoutWriter.Close()
	stderrWriter.Close()

	if err != nil {
		stdoutReader.Close()
		stderrReader.Close()

		return fmt.Errorf("failed to start %s: %w", command, err)
	}

	var output sync.WaitGroup
	output.Add(2)

	go func() { defer output.Done(); a.logs.pump(guest.StreamStdout, stdoutReader) }()
	go func() { defer output.Done(); a.logs.pump(guest.StreamStderr, stderrReader) }()

	a.process = &process
	a.pid = pid
	a.status = guest.Status{
		State:      guest.StateRunning,
		Generation: a.status.Generation + 1,
		StartedAt:  time.Now().UTC(),
	}
	a.ended = make(chan struct{})

	go a.collect(a.status.Generation, ended, &output, stdoutReader, stderrReader)

	return nil
}

// collect waits for a run of the task to end. When the process it was started
// as ends, the task ends: whatever it started is ended along with it, as it is
// when a container's init goes, and what it wrote last is read before anybody
// is told.
func (a *Agent) collect(generation uint64, ended <-chan unix.WaitStatus, output *sync.WaitGroup, readers ...*os.File) {
	status := <-ended

	if err := killCgroup(taskCgroup, emptyTimeout); err != nil {
		a.logger.Warn("what the task started could not all be ended", "error", err)
	}

	drained := make(chan struct{})
	go func() { output.Wait(); close(drained) }()

	select {
	case <-drained:
	case <-time.After(drainTimeout):
		// something outside the task holds its output open; what is left
		// of it is let go rather than waited on forever.
		for _, reader := range readers {
			reader.Close()
		}
	}

	a.lock.Lock()
	defer a.lock.Unlock()

	if a.status.Generation != generation {
		return
	}

	a.status.State = guest.StateExited
	a.status.ExitCode = exitCode(status)
	a.status.FinishedAt = time.Now().UTC()
	a.pid = 0

	close(a.ended)
}

// processStatus says what has become of the task.
func (a *Agent) processStatus(rw http.ResponseWriter, r *http.Request) {
	a.lock.Lock()
	status := a.status
	a.lock.Unlock()

	answer(rw, status)
}

// wait answers once the run it was asked about has ended, or at once if it
// already has.
func (a *Agent) wait(rw http.ResponseWriter, r *http.Request) {
	generation, err := strconv.ParseUint(r.URL.Query().Get("generation"), 10, 64)
	if err != nil {
		http.Error(rw, "which run is being waited for", http.StatusBadRequest)

		return
	}

	for {
		a.lock.Lock()
		status, ended := a.status, a.ended
		a.lock.Unlock()

		if status.Generation > generation || status.State == guest.StateExited {
			answer(rw, status)

			return
		}

		if status.State == guest.StateCreated {
			http.Error(rw, "the task has not been started", http.StatusConflict)

			return
		}

		select {
		case <-ended:
		case <-r.Context().Done():
			return
		}
	}
}

// signal sends a signal to the process the task was started as.
func (a *Agent) signal(rw http.ResponseWriter, r *http.Request) {
	var signal guest.Signal
	if !decode(rw, r, &signal) {
		return
	}

	a.lock.Lock()
	pid, running := a.pid, a.status.State == guest.StateRunning
	a.lock.Unlock()

	if !running {
		http.Error(rw, "the task is not running", http.StatusConflict)

		return
	}

	if err := syscall.Kill(pid, syscall.Signal(signal.Signal)); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// stop asks the task to end, gives it the time it was allowed, and then ends
// it and everything it started. A task that is not running has nothing to
// stop, and says what it ended with.
func (a *Agent) stop(rw http.ResponseWriter, r *http.Request) {
	var stop guest.Stop
	if !decode(rw, r, &stop) {
		return
	}

	a.lock.Lock()
	pid, ended, running := a.pid, a.ended, a.status.State == guest.StateRunning
	a.lock.Unlock()

	if running {
		_ = syscall.Kill(pid, syscall.SIGTERM)

		select {
		case <-ended:
		case <-time.After(stop.Timeout):
			if err := killCgroup(taskCgroup, emptyTimeout); err != nil {
				a.logger.Warn("the task could not be ended", "error", err)
			}

			<-ended
		}
	}

	a.lock.Lock()
	status := a.status
	a.lock.Unlock()

	answer(rw, status)
}

func workingDir(dir string) string {
	if len(dir) == 0 {
		return "/"
	}

	return dir
}
