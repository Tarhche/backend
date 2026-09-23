package task

import ()

// Status represents the status of a task
type Status uint

const (
	StatusCreated    Status = 1 // A task that has never been started.
	StatusRunning    Status = 2 // A running task, started by the runtime.
	StatusPaused     Status = 3 // A paused task. Paused by the runtime.
	StatusRestarting Status = 4 // A task which is starting due to the designated restart policy for that task.
	StatusExited     Status = 5 // A task which is no longer running. For example, the process inside the task completed or the task was stopped.
	StatusRemoving   Status = 6 // A task which is in the process of being removed.
	StatusDead       Status = 7 // A "defunct" task; for example, a task that was only partially removed because resources were kept busy by an external process. dead tasks cannot be (re)started, only removed.
)

// Ended reports whether a task has stopped running for good, which is
// when what it returned is worth asking about.
func (s Status) Ended() bool {
	return s == StatusExited || s == StatusRemoving || s == StatusDead
}

// endedBySignal reports whether what was in a task was ended from
// outside rather than by its own doing. A process killed by signal N is
// reported as 128+N, as a shell and every runtime report it, which is what stopping a task, or taking one away
// when its time is up, looks like from here: the code did not fail, it was cut
// short.
func endedBySignal(exitCode int) bool {
	return exitCode >= 128
}

// EvaluateState reads what a task's status means for the task running
// in it.
//
// The same status means different things for the two kinds. A job that exits
// has finished, which is the whole point of running it — unless what was in it
// returned a failure, since a job that ends badly has not completed. A service
// that exits has stopped: it was meant to keep going, so its exit is the end of
// a run rather than the completion of one — and it is something that can be
// started again, which a completed job is not. Whether a service that ended was
// supposed to is not something a task can say, so that is left to whoever
// knows what was asked of it.
//
// The exit code is what the task's process returned. A runtime only tells
// it on inspection, so a task that was merely listed reports none, and a
// task that has not ended has none to report.
func EvaluateState(status Status, kind Kind, exitCode int) State {
	switch status {
	case StatusCreated:
		return Scheduled
	case StatusRunning:
		return Running
	case StatusRestarting:
		return Restarting
	case StatusPaused:
		return Stopped
	case StatusDead:
		return Failed
	case StatusExited, StatusRemoving:
		if kind == KindService {
			return Stopped
		}

		if exitCode != 0 && !endedBySignal(exitCode) {
			return Failed
		}

		return Completed
	default:
		return Failed
	}
}
