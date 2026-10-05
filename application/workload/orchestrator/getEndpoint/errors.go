package getEndpoint

import "errors"

var (
	// ErrNotHeld is reported when this node holds nothing by that slug. The
	// ingress sent the request here because the control plane's record said
	// so, so the two have got out of step rather than the slug naming nothing.
	ErrNotHeld = errors.New("this node is not holding anything by that name")

	// ErrNotRunning is reported when what the slug names is here but down.
	ErrNotRunning = errors.New("it is not running")

	// ErrNotExposed is reported when it exposes no port that can be reached,
	// or not the one that was asked for.
	ErrNotExposed = errors.New("it does not expose that port")
)
