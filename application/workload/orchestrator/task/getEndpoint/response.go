package getEndpoint

import (
	"net"
	"strconv"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Response is where a request to a task goes: one of the ports of the run that
// is serving it, and, when the run's runtime published that port somewhere,
// where.
//
// How a run is reached is its runtime's business. A runtime that reaches its
// runs itself (task.Dialer) is asked for a connection to the run's own port,
// which is how a microVM is reached, since nothing outside it has a route into
// its network. One that cannot is reached at the address it published the port
// at: the docker daemon's host, and the port docker picked.
type Response struct {
	// ExecutionID is the run, as the runtime calls it, and Port the task's
	// own port on it.
	ExecutionID string    `json:"execution_id"`
	Port        port.Port `json:"port"`

	// Host and HostPort are where the port was published, for a runtime that
	// cannot dial its runs itself. HostPort is zero for a port published
	// nowhere; Host is the node's advertised host, and empty is this one.
	Host     string    `json:"host,omitempty"`
	HostPort port.Port `json:"host_port,omitempty"`
}

// Published reports whether the port can be reached without the runtime: at
// the address it was published at.
func (r *Response) Published() bool {
	return r.HostPort > 0
}

// Address is the host:port the port was published at.
func (r *Response) Address() string {
	return net.JoinHostPort(r.Host, strconv.FormatUint(uint64(r.HostPort), 10))
}
