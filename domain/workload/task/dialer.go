package task

import (
	"context"
	"net"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Dialer is a runtime that reaches its runs' ports itself, rather than
// publishing them on an address something else has to be able to route to.
//
// A microVM publishes nothing on its host: nothing outside it has a route into
// its network, which is the point of it. So the node holding it does not look
// up where a port is and connect there; it asks the runtime for a connection,
// and gets a plain byte stream to the port, as the run's neighbours on its own
// network would reach it. Docker's containers can be reached the same way,
// through the host their ports were published on.
//
// It sits next to Runtime rather than inside it, and is found with a type
// assertion, so a runtime that cannot dial still runs tasks.
type Dialer interface {
	// DialContext connects to one of the ports a run exposes. A runtime
	// that holds several kinds of runs, of which this one cannot be dialled,
	// answers ErrNotSupported, so the caller can reach it another way.
	DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error)
}
