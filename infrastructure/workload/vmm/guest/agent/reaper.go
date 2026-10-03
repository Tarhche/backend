//go:build linux

package agent

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// reaper collects every process that ends in the machine.
//
// The agent is init, so it inherits every orphan the task leaves behind, and a
// process nobody collects stays behind as a zombie. So one loop collects them
// all, and hands what it collected to whoever started it. Nothing else may
// wait for a process: whichever waits first takes the status, and the other is
// left waiting forever. That is also why there is one reaper for the whole
// process, however many agents it holds — a test holds several — and why
// nothing in it may start a process any other way.
type reaper struct {
	once sync.Once

	lock    sync.Mutex
	waiting map[int]chan unix.WaitStatus
}

// children is the process's one reaper.
var children = &reaper{waiting: make(map[int]chan unix.WaitStatus)}

// start starts collecting processes, once, for as long as the process runs.
func (r *reaper) start() {
	r.once.Do(func() {
		ended := make(chan os.Signal, 1)
		signal.Notify(ended, syscall.SIGCHLD)

		go func() {
			for range ended {
				r.collect()
			}
		}()
	})
}

// collect takes every process that has ended. Signals are merged when they
// arrive together, so one is taken to mean any number.
func (r *reaper) collect() {
	r.lock.Lock()
	defer r.lock.Unlock()

	for {
		var status unix.WaitStatus

		pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
		if pid <= 0 || err != nil {
			return
		}

		if waiting, found := r.waiting[pid]; found {
			waiting <- status
			delete(r.waiting, pid)
		}
	}
}

// spawn starts a process and hands back where its end will be told. It is
// started with collecting held off, so a process that ends at once is still
// told to whoever started it rather than collected before anybody asked.
func (r *reaper) spawn(start func() (int, error)) (int, <-chan unix.WaitStatus, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	pid, err := start()
	if err != nil {
		return 0, nil, err
	}

	ended := make(chan unix.WaitStatus, 1)
	r.waiting[pid] = ended

	return pid, ended, nil
}

// exitCode is what a process returned, or 128+N when signal N ended it, which
// is how a shell reports it.
func exitCode(status unix.WaitStatus) int {
	if status.Signaled() {
		return 128 + int(status.Signal())
	}

	return status.ExitStatus()
}
