package task

// Status represents the status of a task
type Status uint

const (
	StatusCreated    Status = 1 // A task that has never been started.
	StatusRunning    Status = 2 // A running task, started by either docker start or docker run.
	StatusPaused     Status = 3 // A paused task. See docker pause.
	StatusRestarting Status = 4 // A task which is starting due to the designated restart policy for that task.
	StatusExited     Status = 5 // A task which is no longer running. For example, the process inside the task completed or the task was stopped using the docker stop command.
	StatusRemoving   Status = 6 // A task which is in the process of being removed. See docker rm.
	StatusDead       Status = 7 // A "defunct" task; for example, a task that was only partially removed because resources were kept busy by an external process. dead tasks cannot be (re)started, only removed.
)

// Ended reports whether a task has stopped running for good, which is
// when what it returned is worth asking about.
func (s Status) Ended() bool {
	return s == StatusExited || s == StatusRemoving || s == StatusDead
}
