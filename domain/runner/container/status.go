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

// EvaluateTaskState reads what a container's status means for the task running
// in it.
//
// The same status means different things for the two kinds. A job that exits
// has finished, which is the whole point of running it. A service that exits
// has stopped: it was meant to keep going, so its exit is the end of a run
// rather than the completion of one — and it is something that can be started
// again, which a completed job is not.
func EvaluateTaskState(status Status, kind task.Kind) task.State {
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

		return task.Completed
	default:
		return task.Failed
	}
}
