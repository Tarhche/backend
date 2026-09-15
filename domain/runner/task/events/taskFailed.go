package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const TaskFailedName = "runnerTaskFailed"

type TaskFailed struct {
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	OwnerUUID   string    `json:"owner_uuid,omitempty"`
	ExecutionID string    `json:"execution_id"`
	NodeName    string    `json:"node_name"`
	At          time.Time `json:"failed_at"`

	// Attempt is which try failed, counting from zero, and MaxRetries how many
	// the task was worth. They come back the way they went out, so that
	// whoever decides whether to ask again can count without looking anything
	// up.
	Attempt    int `json:"attempt"`
	MaxRetries int `json:"max_retries"`

	// Reason is why it failed, when the failure is one the runner can explain:
	// a task that could not be created says so, while one that ran and
	// exited badly speaks for itself through its log.
	Reason string `json:"reason,omitempty"`
}

// LastAttempt reports whether this is the end of it: no further attempt is
// coming, so whoever is waiting on the task can be told now.
func (t *TaskFailed) LastAttempt() bool {
	if t.MaxRetries == task.RetryForever {
		return false
	}

	return t.Attempt >= t.MaxRetries
}
