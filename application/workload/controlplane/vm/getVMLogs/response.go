package getVMLogs

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Response is the lines, or why there are none.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// NodeError is why the node holding the VM did not answer.
	NodeError *noderequest.Error `json:"error,omitempty"`

	Lines []noderequest.VMLogLine `json:"lines"`

	// Truncated says there were more lines than a reply carries.
	Truncated bool `json:"truncated"`
}
