package container

import (
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Status represents the status of a container
type Status uint

const (
	StatusCreated    Status = 1 // A container that has never been started.
	StatusRunning    Status = 2 // A running container, started by either docker start or docker run.
	StatusPaused     Status = 3 // A paused container. See docker pause.
	StatusRestarting Status = 4 // A container which is starting due to the designated restart policy for that container.
	StatusExited     Status = 5 // A container which is no longer running. For example, the process inside the container completed or the container was stopped using the docker stop command.
	StatusRemoving   Status = 6 // A container which is in the process of being removed. See docker rm.
	StatusDead       Status = 7 // A "defunct" container; for example, a container that was only partially removed because resources were kept busy by an external process. dead containers cannot be (re)started, only removed.
)

// Ended reports whether a container has stopped running for good, which is
// when what it returned is worth asking about.
func (s Status) Ended() bool {
	return s == StatusExited || s == StatusRemoving || s == StatusDead
}

// endedBySignal reports whether what was in a container was ended from
// outside rather than by its own doing. Docker reports a process killed by
// signal N as 128+N, which is what stopping a container, or taking one away
// when its time is up, looks like from here: the code did not fail, it was cut
// short.
func endedBySignal(exitCode int) bool {
	return exitCode >= 128
}

// EvaluateTaskState reads what a container's status means for the task running
// in it.
//
// The same status means different things for the two kinds. A job that exits
// has finished, which is the whole point of running it — unless what was in it
// returned a failure, since a job that ends badly has not completed. A service
// that exits has stopped: it was meant to keep going, so its exit is the end of
// a run rather than the completion of one — and it is something that can be
// started again, which a completed job is not. Whether a service that ended was
// supposed to is not something a container can say, so that is left to whoever
// knows what was asked of it.
//
// The exit code is what the container's process returned. Docker only tells it
// on inspection, so a container that was merely listed reports none, and a
// container that has not ended has none to report.
func EvaluateTaskState(status Status, kind task.Kind, exitCode int) task.State {
	switch status {
	case StatusCreated:
		return task.Scheduled
	case StatusRunning:
		return task.Running
	case StatusRestarting:
		return task.Restarting
	case StatusPaused:
		return task.Stopped
	case StatusDead:
		return task.Failed
	case StatusExited, StatusRemoving:
		if kind == task.KindService {
			return task.Stopped
		}

		if exitCode != 0 && !endedBySignal(exitCode) {
			return task.Failed
		}

		return task.Completed
	default:
		return task.Failed
	}
}
