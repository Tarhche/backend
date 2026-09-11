package getEndpoint

import "errors"

var (
	// ErrNotHeld is reported when this node is holding no container by that
	// slug. The ingress sent the request here because the manager's record said
	// so, so the two have got out of step rather than the container not
	// existing at all.
	ErrNotHeld = errors.New("this node is not holding that container")

	// ErrNotRunning is reported when the container is here but stopped.
	ErrNotRunning = errors.New("the container is not running")

	// ErrNotExposed is reported when the container exposes no port that can be
	// reached, or not the one that was asked for.
	ErrNotExposed = errors.New("the container does not expose that port")
)
