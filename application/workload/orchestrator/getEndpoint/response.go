package getEndpoint

import "github.com/khanzadimahdi/testproject/domain/workload/port"

// Response is where what was asked for can be reached from this node.
type Response struct {
	// Port is the guest port that answers, which is the lowest one exposed
	// when none was named.
	Port port.Port `json:"port"`

	// Address is the host:port a request to it is sent to.
	Address string `json:"address"`
}
