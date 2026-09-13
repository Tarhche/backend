package getEndpoint

import "errors"

var (
	// ErrNotHeld is reported when this node is holding no task by that
	// slug. The ingress sent the request here because the manager's record said
	// so, so the two have got out of step rather than the task not
	// existing at all.
	ErrNotHeld = errors.New("this node is not holding that task")

	// ErrNotRunning is reported when the task is here but stopped.
	ErrNotRunning = errors.New("the task is not running")

	// ErrNotExposed is reported when the task exposes no port that can be
	// reached, or not the one that was asked for.
	ErrNotExposed = errors.New("the task does not expose that port")
)
