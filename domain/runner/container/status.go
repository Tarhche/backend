package container

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
