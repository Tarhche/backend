package getEndpoint

import (
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// Response is which run of the task a request reaches, and on which of its
// ports. Where that run is, and how this node gets to it, is the runtime's to
// know: a request is carried to it by dialling through the runtime.
type Response struct {
	ExecutionID string    `json:"execution_id"`
	Port        port.Port `json:"port"`
}
