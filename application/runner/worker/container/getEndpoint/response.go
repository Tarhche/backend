package getEndpoint

import (
	"net"
	"strconv"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// Response is where the container can actually be reached from this node.
type Response struct {
	Host string    `json:"host"`
	Port port.Port `json:"port"`
}

// Address is the host:port a request to the container is sent to.
func (r *Response) Address() string {
	return net.JoinHostPort(r.Host, strconv.FormatUint(uint64(r.Port), 10))
}
