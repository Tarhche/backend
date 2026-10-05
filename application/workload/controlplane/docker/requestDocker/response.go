package requestDocker

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Response is what dockerd answered, or why it did not.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// NodeError is why there is no answer: the VM is not a Docker VM, or not
	// running, or its node refused or did not answer.
	NodeError *noderequest.Error `json:"error,omitempty"`

	// Result is the operation's own answer, as the node gave it.
	Result json.RawMessage `json:"result,omitempty"`

	// Truncated says the result was cut to fit in a reply.
	Truncated bool `json:"truncated,omitempty"`
}
