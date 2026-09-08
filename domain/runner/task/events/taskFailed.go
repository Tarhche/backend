package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const TaskFailedName = "runnerTaskFailed"

type TaskFailed struct {
	UUID          string    `json:"uuid"`
	ContainerUUID string    `json:"container_uuid"`
	NodeName      string    `json:"node_name"`
	At            time.Time `json:"failed_at"`

	// Name is what the task was called, so that whoever asked for it can be
	// told what became of it without looking it up first.
	Name string `json:"name,omitempty"`

	// Attempt is which try failed, counting from zero, and MaxRetries how many
	// the container was worth. They come back the way they went out, so that
	// whoever decides whether to ask again can count without looking anything
	// up.
	Attempt    int `json:"attempt"`
	MaxRetries int `json:"max_retries"`

	// Reason is why it failed, when the failure is one the runner can explain:
	// a container that could not be created says so, while one that ran and
	// exited badly speaks for itself through its log.
	Reason string `json:"reason,omitempty"`
}

// LastAttempt reports whether this is the end of it: no further attempt is
// coming, so whoever is waiting on the container can be told now.
func (t *TaskFailed) LastAttempt() bool {
	if t.MaxRetries == task.RetryForever {
		return false
	}

	return t.Attempt >= t.MaxRetries
}
